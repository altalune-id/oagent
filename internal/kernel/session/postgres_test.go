package session

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/sealer"
)

type unexpectedCalls struct{ n int }

func (u *unexpectedCalls) fn(_ context.Context, msg string, cause error, _ ...any) *apperror.AppError {
	u.n++
	return apperror.New(apperror.CodeUnexpected, msg).WithCause(cause)
}

func newMockPostgresStore(t *testing.T) (*PostgresStore, sqlmock.Sqlmock, *unexpectedCalls) {
	t.Helper()
	conn, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	calls := &unexpectedCalls{}
	return NewPostgresStore(conn, "public", "", newSealer(t), calls.fn), mock, calls
}

func TestPostgresStore_LoadMissingIsSignedOut(t *testing.T) {
	s, mock, _ := newMockPostgresStore(t)
	mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows([]string{"sessions.payload"}))
	p, ok, err := s.Load(context.Background(), "sid-1")
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Equal(t, Principal{}, p)
}

func TestPostgresStore_LoadOpensPayload(t *testing.T) {
	s, mock, _ := newMockPostgresStore(t)
	want := Principal{UserID: uuid.New(), Email: "a@example.com", Source: SourceLocal}
	blob, err := sealPrincipal(s.sealer, "sid-1", want)
	require.NoError(t, err)
	mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows([]string{"sessions.payload"}).AddRow(blob))
	got, ok, err := s.Load(context.Background(), "sid-1")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, want.UserID, got.UserID)
	assert.Equal(t, want.Email, got.Email)
}

func TestPostgresStore_UnopenablePayload(t *testing.T) {
	s, mock, calls := newMockPostgresStore(t)
	mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows([]string{"sessions.payload"}).AddRow([]byte("garbage")))
	p, ok, err := s.Load(context.Background(), "sid-1")
	require.NoError(t, err, "an unopenable session must not become a 500")
	assert.False(t, ok, "it reads as signed out")
	assert.Equal(t, Principal{}, p)
	assert.Equal(t, 1, calls.n, "it is reported, because a rotated key signs everyone out")
}

func TestPostgresStore_Errors(t *testing.T) {
	s, mock, calls := newMockPostgresStore(t)
	boom := errors.New("boom")
	mock.ExpectQuery("SELECT").WillReturnError(boom)
	mock.ExpectExec("INSERT").WillReturnError(boom)
	mock.ExpectExec("DELETE").WillReturnError(boom)
	mock.ExpectExec("DELETE").WillReturnError(boom)
	mock.ExpectExec("DELETE").WillReturnError(boom)
	ctx := context.Background()

	_, _, err := s.Load(ctx, "sid-1")
	assert.ErrorIs(t, err, boom)
	assert.ErrorIs(t, s.Save(ctx, "sid-1", Principal{UserID: uuid.New()}, time.Now().Add(time.Hour)), boom)
	assert.ErrorIs(t, s.Delete(ctx, "sid-1"), boom)
	assert.ErrorIs(t, s.DeleteByUser(ctx, uuid.New()), boom)
	_, err = s.DeleteExpired(ctx)
	assert.ErrorIs(t, err, boom)
	assert.Zero(t, calls.n, "store errors are returned, not reported here")
}

func TestPostgresStore_DeleteExpiredLoopsOverBatches(t *testing.T) {
	s, mock, _ := newMockPostgresStore(t)
	s.batch = 2
	mock.ExpectExec("DELETE").WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec("DELETE").WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec("DELETE").WillReturnResult(sqlmock.NewResult(0, 1))
	n, err := s.DeleteExpired(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestPostgresStore_SaveSealError(t *testing.T) {
	conn, _, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	calls := &unexpectedCalls{}
	s := NewPostgresStore(conn, "public", "", sealer.Disabled(), calls.fn)
	assert.Error(t, s.Save(context.Background(), "sid-1", Principal{UserID: uuid.New()}, time.Now().Add(time.Hour)))
}
