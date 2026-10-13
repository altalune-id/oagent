package mailqueue

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/mailer"
	"altalune.id/oagent/sealer"
)

type capture struct{ v driver.Value }

func (c *capture) Match(v driver.Value) bool { c.v = v; return true }

func newTestStore(t *testing.T, sl sealer.Sealer, allowPlain bool) (*postgresStore, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return NewStore(db.Pool{W: sqlDB, R: sqlDB}, "public", "", sl, allowPlain), mock, sqlDB
}

func testSealer(t *testing.T) sealer.Sealer {
	t.Helper()
	key, err := sealer.GenerateKey()
	require.NoError(t, err)
	sl, err := sealer.New(key)
	require.NoError(t, err)
	return sl
}

type brokenSealer struct{}

func (brokenSealer) Seal(_, _ []byte) ([]byte, error) { return nil, errors.New("hsm offline") }
func (brokenSealer) Open(_, _ []byte) ([]byte, error) { return nil, errors.New("hsm offline") }

func expectInsert(mock sqlmock.Sqlmock, id, payload, sealed *capture) {
	mock.ExpectExec(`INSERT INTO public.mail_queue`).
		WithArgs(id, payload, sealed, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func TestEnqueueRejectsAnInvalidMessage(t *testing.T) {
	s, mock, _ := newTestStore(t, testSealer(t), false)
	m := testMessage()
	m.Subject = "Hi\r\nBcc: x@example.com"
	_, err := s.Enqueue(context.Background(), m)
	assert.True(t, mailer.IsHeaderInjectionError(err))

	_, err = s.Enqueue(context.Background(), mailer.Message{Subject: "s", TextBody: "b"})
	assert.True(t, mailer.IsIncompleteMessageError(err))
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestEnqueueSealsThePayload(t *testing.T) {
	s, mock, _ := newTestStore(t, testSealer(t), true)
	id, payload, sealed := &capture{}, &capture{}, &capture{}
	expectInsert(mock, id, payload, sealed)

	got, err := s.Enqueue(context.Background(), testMessage())
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())

	stored, ok := payload.v.([]byte)
	require.True(t, ok)
	assert.Equal(t, true, sealed.v)
	assert.False(t, json.Valid(stored), "a sealed payload is not plain JSON")
	assert.NotContains(t, string(stored), testTo)
	assert.NotContains(t, string(stored), "tok_abcdef123456")

	msg, err := s.Open(Entry{ID: got, Payload: stored, Sealed: true})
	require.NoError(t, err)
	assert.Equal(t, testMessage(), msg)

	_, err = s.Open(Entry{ID: uuid.New(), Payload: stored, Sealed: true})
	assert.True(t, IsPayloadError(err), "the seal is bound to the entry id")
}

func TestEnqueueRequiresASealerUnlessPlainIsAllowed(t *testing.T) {
	s, mock, _ := newTestStore(t, sealer.Disabled(), false)
	_, err := s.Enqueue(context.Background(), testMessage())
	assert.True(t, IsSealerRequiredError(err))
	assert.NotContains(t, err.Error(), testTo)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestEnqueueStoresPlainOnlyWhenAllowed(t *testing.T) {
	s, mock, _ := newTestStore(t, sealer.Disabled(), true)
	id, payload, sealed := &capture{}, &capture{}, &capture{}
	expectInsert(mock, id, payload, sealed)

	got, err := s.Enqueue(context.Background(), testMessage())
	require.NoError(t, err)
	assert.Equal(t, false, sealed.v)
	stored, _ := payload.v.([]byte)
	assert.True(t, json.Valid(stored))

	msg, err := s.Open(Entry{ID: got, Payload: stored})
	require.NoError(t, err)
	assert.Equal(t, testMessage(), msg)
}

func TestEnqueueSealerFailure(t *testing.T) {
	s, mock, _ := newTestStore(t, brokenSealer{}, true)
	_, err := s.Enqueue(context.Background(), testMessage())
	require.Error(t, err)
	assert.False(t, IsSealerRequiredError(err))
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestEnqueueInsertError(t *testing.T) {
	s, mock, _ := newTestStore(t, testSealer(t), false)
	mock.ExpectExec(`INSERT INTO public.mail_queue`).WillReturnError(errors.New("boom"))
	_, err := s.Enqueue(context.Background(), testMessage())
	assert.ErrorContains(t, err, "boom")
}

func TestEnqueueJoinsTheCallersTransaction(t *testing.T) {
	s, mock, sqlDB := newTestStore(t, testSealer(t), false)
	mock.ExpectBegin()
	tx, err := sqlDB.Begin()
	require.NoError(t, err)
	expectInsert(mock, &capture{}, &capture{}, &capture{})
	mock.ExpectRollback()

	_, err = s.Enqueue(db.ContextWithTx(context.Background(), tx), testMessage())
	require.NoError(t, err)
	require.NoError(t, tx.Rollback())
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestOpenRejectsBadPayloads(t *testing.T) {
	s, _, _ := newTestStore(t, testSealer(t), false)
	_, err := s.Open(Entry{ID: uuid.New()})
	assert.True(t, IsPayloadError(err))

	_, err = s.Open(Entry{ID: uuid.New(), Payload: []byte("not json")})
	assert.True(t, IsPayloadError(err))
	assert.NotContains(t, err.Error(), "not json")
}

func TestClaimDueBeginError(t *testing.T) {
	s, mock, _ := newTestStore(t, testSealer(t), false)
	mock.ExpectBegin().WillReturnError(errors.New("down"))
	_, err := s.ClaimDue(context.Background(), now(), 1)
	assert.ErrorContains(t, err, "down")
}

var errBoom = errors.New("boom")

func now() time.Time { return time.Now().UTC() }
