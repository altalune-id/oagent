//go:build integration

package legal_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/legal"
	"altalune.id/oagent/internal/testutil/pgtest"
	"altalune.id/oagent/internal/user"
)

func TestPostgresLegal_PublishAndRecord(t *testing.T) {
	sqlDB, schema := pgtest.Migrated(t)
	pool := db.Pool{W: sqlDB, R: sqlDB}
	ctx := t.Context()
	users := user.NewService(user.NewStore(pool, schema, ""), apperror.NewReporter(nil, false).Unexpected)
	store := legal.NewStore(pool, schema, "")
	svc := legal.NewService(users).WithStore(store, db.NewTransactor(pool))
	require.NoError(t, svc.Publish(ctx))
	first := svc.TermsUpdatedAt()
	assert.False(t, first.IsZero())

	later := legal.NewService(users).WithStore(store, db.NewTransactor(pool))
	require.NoError(t, later.Publish(ctx))
	assert.Equal(t, first, later.TermsUpdatedAt(), "a second boot keeps the first publish time")

	u, err := users.Create(ctx, user.CreateRequest{Email: "ada@example.com", Password: "a sturdy passphrase"})
	require.NoError(t, err)
	at, err := svc.Accept(ctx, u.ID, legal.SourceWelcome)
	require.NoError(t, err)
	var n int
	var source string
	var acceptedAt time.Time
	require.NoError(t, sqlDB.QueryRowContext(ctx, `SELECT count(*), max(source), max(accepted_at) FROM terms_acceptances WHERE user_id = $1`, u.ID).
		Scan(&n, &source, &acceptedAt))
	assert.Equal(t, 1, n)
	assert.Equal(t, "welcome", source)
	assert.WithinDuration(t, at, acceptedAt, time.Microsecond)
	var body string
	require.NoError(t, sqlDB.QueryRowContext(ctx, `SELECT d.body FROM terms_acceptances a JOIN legal_documents d ON d.id = a.terms_document_id WHERE a.user_id = $1`, u.ID).Scan(&body))
	assert.Contains(t, body, "## 1.", "the exact text accepted is kept")

	_, err = sqlDB.ExecContext(ctx, `UPDATE legal_documents SET sha256 = 'other' WHERE slug = 'terms'`)
	require.NoError(t, err)
	assert.Error(t, legal.NewService(users).WithStore(store, db.NewTransactor(pool)).Publish(ctx), "changed text under a version refuses to boot")

	err = store.RecordAcceptance(ctx, legal.Acceptance{ID: uuid.New(), PublicID: "x", UserID: u.ID, TermsDocumentID: uuid.New(), PrivacyDocumentID: uuid.New(), Source: legal.SourceJoin, AcceptedAt: at})
	pgErr, ok := errors.AsType[*pgconn.PgError](err)
	require.True(t, ok, "%v", err)
	assert.Equal(t, "23503", pgErr.Code, "an acceptance always names stored documents")
}

func TestPostgresLegal_TimesFollowTheDatabaseClock(t *testing.T) {
	sqlDB, schema := pgtest.Migrated(t)
	pool := db.Pool{W: sqlDB, R: sqlDB}
	ctx := t.Context()
	users := user.NewService(user.NewStore(pool, schema, ""), apperror.NewReporter(nil, false).Unexpected)
	svc := legal.NewService(users).WithStore(legal.NewStore(pool, schema, ""), db.NewTransactor(pool))
	u, err := users.Create(ctx, user.CreateRequest{Email: "ada@example.com", Password: "a sturdy passphrase"})
	require.NoError(t, err)

	dbNow := func(ctx context.Context) time.Time {
		tx, ok := db.CurrentTx(ctx)
		require.True(t, ok)
		var now time.Time
		require.NoError(t, tx.QueryRowContext(ctx, `SELECT now()`).Scan(&now))
		time.Sleep(20 * time.Millisecond)
		return now.UTC()
	}
	var published, accepted, at time.Time
	require.NoError(t, db.RunInTx(ctx, pool, func(ctx context.Context) error {
		published = dbNow(ctx)
		return svc.Publish(ctx)
	}))
	assert.True(t, published.Equal(svc.TermsUpdatedAt()), "published_at is the database time, not the app's: %v vs %v", published, svc.TermsUpdatedAt())

	require.NoError(t, db.RunInTx(ctx, pool, func(ctx context.Context) error {
		accepted = dbNow(ctx)
		at, err = svc.Accept(ctx, u.ID, legal.SourceWelcome)
		return err
	}))
	assert.True(t, accepted.Equal(at), "terms_accepted_at is the database time, not the app's: %v vs %v", accepted, at)
	stored, err := users.ByID(ctx, u.ID)
	require.NoError(t, err)
	require.NotNil(t, stored.TermsAcceptedAt)
	assert.True(t, accepted.Equal(*stored.TermsAcceptedAt))
	assert.False(t, svc.NeedsAcceptance(at), "accepting passes the gate")
}
