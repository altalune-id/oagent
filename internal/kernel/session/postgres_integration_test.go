//go:build integration

package session_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/kernel/session/sessiontest"
	"altalune.id/oagent/internal/testutil/pgtest"
	"altalune.id/oagent/sealer"
)

type withUsers struct {
	*session.PostgresStore
	db *sql.DB
}

func (s withUsers) Save(ctx context.Context, sid string, p session.Principal, expiresAt time.Time) error {
	if err := insertUser(ctx, s.db, p.UserID); err != nil {
		return err
	}
	return s.PostgresStore.Save(ctx, sid, p, expiresAt)
}

func insertUser(ctx context.Context, db *sql.DB, id uuid.UUID) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO users (id, public_id, email, created_at, updated_at) VALUES ($1, $2, $3, now(), now()) ON CONFLICT (id) DO NOTHING`,
		id, "u-"+id.String(), id.String()+"@example.com")
	return err
}

type reported struct{ n int }

func (r *reported) fn(_ context.Context, msg string, cause error, _ ...any) *apperror.AppError {
	r.n++
	return apperror.New(apperror.CodeUnexpected, msg).WithCause(cause)
}

func newPGSessionStore(t *testing.T) (*session.PostgresStore, *sql.DB, *reported) {
	t.Helper()
	sqlDB, schemaName := pgtest.Migrated(t)
	key, err := sealer.GenerateKey()
	require.NoError(t, err)
	sl, err := sealer.New(key)
	require.NoError(t, err)
	r := &reported{}
	return session.NewPostgresStore(sqlDB, schemaName, "", sl, r.fn), sqlDB, r
}

func TestPostgresStore_Contract(t *testing.T) {
	sessiontest.RunStoreContract(t, func(t *testing.T) session.Store {
		s, sqlDB, _ := newPGSessionStore(t)
		return withUsers{PostgresStore: s, db: sqlDB}
	})
}

func TestPostgresStore_DeleteExpiredBatches(t *testing.T) {
	s, sqlDB, _ := newPGSessionStore(t)
	store := withUsers{PostgresStore: s, db: sqlDB}
	session.SetBatch(s, 2)
	ctx := t.Context()
	for i := range 5 {
		require.NoError(t, store.Save(ctx, "old-"+string(rune('a'+i)), session.Principal{UserID: uuid.New()}, time.Now().Add(-time.Minute)))
	}
	require.NoError(t, store.Save(ctx, "live", session.Principal{UserID: uuid.New()}, time.Now().Add(time.Hour)))

	n, err := s.DeleteExpired(ctx)
	require.NoError(t, err)
	assert.Equal(t, 5, n, "every batch is drained")
	_, ok, err := s.Load(ctx, "live")
	require.NoError(t, err)
	assert.True(t, ok)
	var left int
	require.NoError(t, sqlDB.QueryRowContext(ctx, `SELECT count(*) FROM sessions`).Scan(&left))
	assert.Equal(t, 1, left)
}

func TestPostgresStore_CopiedRowReadsAsSignedOut(t *testing.T) {
	s, sqlDB, r := newPGSessionStore(t)
	store := withUsers{PostgresStore: s, db: sqlDB}
	ctx := t.Context()
	p := session.Principal{UserID: uuid.New(), Source: session.SourceLocal}
	require.NoError(t, store.Save(ctx, "victim", p, time.Now().Add(time.Hour)))
	_, err := sqlDB.ExecContext(ctx,
		`INSERT INTO sessions (sid, user_id, payload, expires_at, created_at)
		 SELECT 'attacker', user_id, payload, expires_at, created_at FROM sessions WHERE sid = 'victim'`)
	require.NoError(t, err)

	got, ok, err := s.Load(ctx, "attacker")
	require.NoError(t, err, "a copied payload must not become a 500")
	assert.False(t, ok, "the payload is sealed to its own sid")
	assert.Equal(t, session.Principal{}, got)
	assert.Equal(t, 1, r.n, "it is reported")
	_, ok, err = s.Load(ctx, "victim")
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestPostgresStore_UserDeleteCascades(t *testing.T) {
	s, sqlDB, _ := newPGSessionStore(t)
	store := withUsers{PostgresStore: s, db: sqlDB}
	ctx := t.Context()
	gone, kept := uuid.New(), uuid.New()
	require.NoError(t, store.Save(ctx, "gone", session.Principal{UserID: gone}, time.Now().Add(time.Hour)))
	require.NoError(t, store.Save(ctx, "kept", session.Principal{UserID: kept}, time.Now().Add(time.Hour)))

	_, err := sqlDB.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, gone)
	require.NoError(t, err)
	_, ok, err := s.Load(ctx, "gone")
	require.NoError(t, err)
	assert.False(t, ok, "deleting a user deletes their sessions")
	_, ok, err = s.Load(ctx, "kept")
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestPostgresStore_RequiresARealUser(t *testing.T) {
	s, _, _ := newPGSessionStore(t)
	for name, p := range map[string]session.Principal{
		"basic credential has no user": {Source: session.SourceBasic},
		"unknown user":                 {UserID: uuid.New(), Source: session.SourceLocal},
	} {
		t.Run(name, func(t *testing.T) {
			err := s.Save(t.Context(), "sid-"+name, p, time.Now().Add(time.Hour))
			pgErr, ok := errors.AsType[*pgconn.PgError](err)
			require.True(t, ok, "got %v", err)
			assert.Equal(t, "23503", pgErr.Code)
		})
	}
}

func TestPostgresStore_SaveRefreshesExpiry(t *testing.T) {
	s, sqlDB, _ := newPGSessionStore(t)
	store := withUsers{PostgresStore: s, db: sqlDB}
	ctx := t.Context()
	p := session.Principal{UserID: uuid.New()}
	require.NoError(t, store.Save(ctx, "sid", p, time.Now().Add(-time.Minute)))
	_, ok, err := s.Load(ctx, "sid")
	require.NoError(t, err)
	require.False(t, ok)

	require.NoError(t, store.Save(ctx, "sid", p, time.Now().Add(time.Hour)))
	_, ok, err = s.Load(ctx, "sid")
	require.NoError(t, err)
	assert.True(t, ok, "a second Save extends the same sid")
}

func TestPostgresStore_DeleteByUserJoinsTheTransaction(t *testing.T) {
	s, sqlDB, _ := newPGSessionStore(t)
	store := withUsers{PostgresStore: s, db: sqlDB}
	ctx := t.Context()
	p := session.Principal{UserID: uuid.New()}
	require.NoError(t, store.Save(ctx, "kept", p, time.Now().Add(time.Hour)))

	rollback := errors.New("rollback")
	err := db.RunInTx(ctx, db.Pool{W: sqlDB, R: sqlDB}, func(ctx context.Context) error {
		require.NoError(t, s.DeleteByUser(ctx, p.UserID))
		return rollback
	})
	require.ErrorIs(t, err, rollback)
	_, ok, err := s.Load(ctx, "kept")
	require.NoError(t, err)
	assert.True(t, ok, "a rolled back removal keeps the session")
}
