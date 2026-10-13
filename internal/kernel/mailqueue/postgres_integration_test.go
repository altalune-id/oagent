//go:build integration

package mailqueue_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/mailqueue"
	"altalune.id/oagent/internal/testutil/pgtest"
	"altalune.id/oagent/mailer"
	"altalune.id/oagent/sealer"
)

type fixture struct {
	db    *sql.DB
	pool  db.Pool
	store interface {
		mailqueue.Queue
		mailqueue.WorkerStore
	}
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	sqlDB, schemaName := pgtest.Migrated(t)
	key, err := sealer.GenerateKey()
	require.NoError(t, err)
	sl, err := sealer.New(key)
	require.NoError(t, err)
	pool := db.Pool{W: sqlDB, R: sqlDB}
	return &fixture{db: sqlDB, pool: pool, store: mailqueue.NewStore(pool, schemaName, "", sl, false)}
}

func message() mailer.Message {
	return mailer.Message{To: "carol@example.com", Subject: "Hello", TextBody: "link https://example.com/t/secret-token"}
}

type row struct {
	payload   []byte
	sealed    bool
	status    string
	attempt   int
	lastError string
	settled   sql.NullTime
	next      time.Time
}

func (f *fixture) row(t *testing.T, id uuid.UUID) row {
	t.Helper()
	var r row
	require.NoError(t, f.db.QueryRowContext(t.Context(),
		`SELECT payload, sealed, status, attempt, last_error, settled_at, next_attempt_at FROM mail_queue WHERE id = $1`, id).
		Scan(&r.payload, &r.sealed, &r.status, &r.attempt, &r.lastError, &r.settled, &r.next))
	return r
}

func (f *fixture) count(t *testing.T) int {
	t.Helper()
	var n int
	require.NoError(t, f.db.QueryRowContext(t.Context(), `SELECT count(*) FROM mail_queue`).Scan(&n))
	return n
}

func later() time.Time { return time.Now().UTC().Add(time.Second) }

func TestEnqueue_StoresASealedPendingRow(t *testing.T) {
	f := newFixture(t)
	id, err := f.store.Enqueue(t.Context(), message())
	require.NoError(t, err)

	r := f.row(t, id)
	assert.Equal(t, "pending", r.status)
	assert.True(t, r.sealed)
	assert.False(t, json.Valid(r.payload))
	assert.NotContains(t, string(r.payload), "secret-token")
	assert.NotContains(t, string(r.payload), "carol@example.com")
}

func TestEnqueue_RollsBackWithTheCallersTransaction(t *testing.T) {
	f := newFixture(t)
	errAbort := errors.New("abort")
	err := db.RunInTx(t.Context(), f.pool, func(ctx context.Context) error {
		if _, err := f.store.Enqueue(ctx, message()); err != nil {
			return err
		}
		return errAbort
	})
	require.ErrorIs(t, err, errAbort)
	assert.Zero(t, f.count(t))

	require.NoError(t, db.RunInTx(t.Context(), f.pool, func(ctx context.Context) error {
		_, err := f.store.Enqueue(ctx, message())
		return err
	}))
	assert.Equal(t, 1, f.count(t))
}

func TestClaimDue_OpenSucceedErasesThePayload(t *testing.T) {
	f := newFixture(t)
	id, err := f.store.Enqueue(t.Context(), message())
	require.NoError(t, err)

	got, err := f.store.ClaimDue(t.Context(), later(), 10)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, id, got[0].ID)
	assert.Equal(t, 1, got[0].Attempt)

	msg, err := f.store.Open(got[0])
	require.NoError(t, err)
	assert.Equal(t, message(), msg)

	require.NoError(t, f.store.Succeed(t.Context(), got[0], time.Now()))
	r := f.row(t, id)
	assert.Equal(t, "sent", r.status)
	assert.Nil(t, r.payload)
	assert.True(t, r.settled.Valid)

	assert.True(t, mailqueue.IsSettleRefusedError(f.store.Succeed(t.Context(), got[0], time.Now())))
}

func TestFail_KeepsPendingThenFailsForGood(t *testing.T) {
	f := newFixture(t)
	id, err := f.store.Enqueue(t.Context(), message())
	require.NoError(t, err)

	got, err := f.store.ClaimDue(t.Context(), later(), 10)
	require.NoError(t, err)
	retryAt := time.Now().UTC().Add(time.Hour)
	require.NoError(t, f.store.Fail(t.Context(), got[0], retryAt, "network: dial"))

	r := f.row(t, id)
	assert.Equal(t, "pending", r.status)
	assert.NotNil(t, r.payload)
	assert.Equal(t, "network: dial", r.lastError)
	assert.WithinDuration(t, retryAt, r.next, time.Millisecond)

	none, err := f.store.ClaimDue(t.Context(), later(), 10)
	require.NoError(t, err)
	assert.Empty(t, none, "backoff holds the entry until retryAt")

	_, err = f.db.ExecContext(t.Context(), `UPDATE mail_queue SET attempt = $2, next_attempt_at = now() WHERE id = $1`, id, mailqueue.MaxAttempts-1)
	require.NoError(t, err)
	last, err := f.store.ClaimDue(t.Context(), later(), 10)
	require.NoError(t, err)
	require.Len(t, last, 1)
	assert.Equal(t, mailqueue.MaxAttempts, last[0].Attempt)
	require.NoError(t, f.store.Fail(t.Context(), last[0], retryAt, "smtp_reply 550"))

	r = f.row(t, id)
	assert.Equal(t, "failed", r.status)
	assert.Nil(t, r.payload)
	assert.True(t, r.settled.Valid)
}

func TestFail_TruncatesTheCause(t *testing.T) {
	f := newFixture(t)
	id, err := f.store.Enqueue(t.Context(), message())
	require.NoError(t, err)
	got, err := f.store.ClaimDue(t.Context(), later(), 10)
	require.NoError(t, err)
	long := make([]byte, 4*mailqueue.MaxErrorLen)
	for i := range long {
		long[i] = 'x'
	}
	require.NoError(t, f.store.Fail(t.Context(), got[0], time.Now(), string(long)))
	assert.Len(t, f.row(t, id).lastError, mailqueue.MaxErrorLen)
}

func TestClaimDue_LeaseExpiryMakesTheRowClaimableAgain(t *testing.T) {
	f := newFixture(t)
	id, err := f.store.Enqueue(t.Context(), message())
	require.NoError(t, err)

	first, err := f.store.ClaimDue(t.Context(), later(), 10)
	require.NoError(t, err)
	require.Len(t, first, 1)

	none, err := f.store.ClaimDue(t.Context(), later(), 10)
	require.NoError(t, err)
	assert.Empty(t, none, "a leased row is not claimable")

	again, err := f.store.ClaimDue(t.Context(), time.Now().UTC().Add(mailqueue.ClaimLease+time.Minute), 10)
	require.NoError(t, err)
	require.Len(t, again, 1)
	assert.Equal(t, id, again[0].ID)
	assert.Equal(t, 2, again[0].Attempt)

	assert.True(t, mailqueue.IsSettleRefusedError(f.store.Succeed(t.Context(), first[0], time.Now())),
		"a superseded claim cannot settle")
	require.NoError(t, f.store.Succeed(t.Context(), again[0], time.Now()))
}

func TestClaimDue_ReapsAnExhaustedLease(t *testing.T) {
	f := newFixture(t)
	id, err := f.store.Enqueue(t.Context(), message())
	require.NoError(t, err)
	_, err = f.db.ExecContext(t.Context(), `UPDATE mail_queue SET attempt = $2 WHERE id = $1`, id, mailqueue.MaxAttempts)
	require.NoError(t, err)

	got, err := f.store.ClaimDue(t.Context(), later(), 10)
	require.NoError(t, err)
	assert.Empty(t, got)
	r := f.row(t, id)
	assert.Equal(t, "failed", r.status)
	assert.Nil(t, r.payload)
	assert.True(t, r.settled.Valid)
}

func TestClaimDue_TwoClaimersNeverShareARow(t *testing.T) {
	f := newFixture(t)
	const total = 40
	for range total {
		_, err := f.store.Enqueue(t.Context(), message())
		require.NoError(t, err)
	}

	var (
		mu   sync.Mutex
		seen = map[uuid.UUID]int{}
		wg   sync.WaitGroup
	)
	start := make(chan struct{})
	for range 4 {
		wg.Go(func() {
			<-start
			for {
				got, err := f.store.ClaimDue(context.Background(), later(), 3)
				if err != nil || len(got) == 0 {
					return
				}
				mu.Lock()
				for _, e := range got {
					seen[e.ID]++
				}
				mu.Unlock()
			}
		})
	}
	close(start)
	wg.Wait()

	assert.Len(t, seen, total)
	for id, n := range seen {
		assert.Equal(t, 1, n, "entry %s claimed twice", id)
	}
}

func TestStore_PlainWhenAllowed(t *testing.T) {
	sqlDB, schemaName := pgtest.Migrated(t)
	pool := db.Pool{W: sqlDB, R: sqlDB}
	s := mailqueue.NewStore(pool, schemaName, "", sealer.Disabled(), true)
	id, err := s.Enqueue(t.Context(), message())
	require.NoError(t, err)

	got, err := s.ClaimDue(t.Context(), later(), 10)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, id, got[0].ID)
	assert.False(t, got[0].Sealed)
	msg, err := s.Open(got[0])
	require.NoError(t, err)
	assert.Equal(t, message(), msg)
}

func TestWorker_EndToEnd(t *testing.T) {
	f := newFixture(t)
	id, err := f.store.Enqueue(t.Context(), message())
	require.NoError(t, err)

	m := &recordingMailer{}
	w := mailqueue.NewWorker(f.store, m, nil, mailqueue.WorkerOpts{Tick: 10 * time.Millisecond})
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()
	require.Eventually(t, func() bool { return f.row(t, id).status == "sent" }, 5*time.Second, 10*time.Millisecond)
	cancel()
	require.NoError(t, <-done)
	assert.Equal(t, 1, m.count())
}

type recordingMailer struct {
	mu sync.Mutex
	n  int
}

func (r *recordingMailer) Send(context.Context, mailer.Message) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.n++
	return nil
}

func (r *recordingMailer) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.n
}

func TestGiveUp_SettlesFailedOnTheFirstAttempt(t *testing.T) {
	f := newFixture(t)
	id, err := f.store.Enqueue(t.Context(), message())
	require.NoError(t, err)
	got, err := f.store.ClaimDue(t.Context(), later(), 10)
	require.NoError(t, err)
	require.Len(t, got, 1)

	at := time.Now().UTC().Truncate(time.Microsecond)
	require.NoError(t, f.store.GiveUp(t.Context(), got[0], at, "http_status 422"))
	r := f.row(t, id)
	assert.Equal(t, "failed", r.status)
	assert.Nil(t, r.payload, "a given-up entry keeps no message")
	assert.Equal(t, "http_status 422", r.lastError)
	assert.WithinDuration(t, at, r.settled.Time, time.Millisecond)
	assert.Equal(t, 1, r.attempt)

	assert.True(t, mailqueue.IsSettleRefusedError(f.store.GiveUp(t.Context(), got[0], at, "again")), "a settled entry cannot be settled twice")
}

func TestPruneSettled_DeletesOnlyOldSettledEntries(t *testing.T) {
	f := newFixture(t)
	pending, err := f.store.Enqueue(t.Context(), message())
	require.NoError(t, err)
	oldSent, err := f.store.Enqueue(t.Context(), message())
	require.NoError(t, err)
	newSent, err := f.store.Enqueue(t.Context(), message())
	require.NoError(t, err)
	_, err = f.db.ExecContext(t.Context(),
		`UPDATE mail_queue SET status = 'sent', payload = NULL, settled_at = now() - interval '40 days' WHERE id = $1`, oldSent)
	require.NoError(t, err)
	_, err = f.db.ExecContext(t.Context(),
		`UPDATE mail_queue SET status = 'failed', payload = NULL, settled_at = now() WHERE id = $1`, newSent)
	require.NoError(t, err)

	n, err := f.store.PruneSettled(t.Context(), time.Now().Add(-mailqueue.Retention))
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)
	assert.Equal(t, 2, f.count(t))
	assert.Equal(t, "pending", f.row(t, pending).status)
	assert.Equal(t, "failed", f.row(t, newSent).status)
}
