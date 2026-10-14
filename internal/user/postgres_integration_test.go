//go:build integration

package user_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/testutil/pgtest"
	"altalune.id/oagent/internal/user"
)

func newPGStore(t *testing.T) (user.Store, db.Pool) {
	t.Helper()
	sqlDB, schemaName := pgtest.Migrated(t)
	pool := db.Pool{W: sqlDB, R: sqlDB}
	return user.NewStore(pool, schemaName, ""), pool
}

func mustNew(t *testing.T, email string) *user.User {
	t.Helper()
	u, err := user.New(email, "Name")
	require.NoError(t, err)
	return u
}

func TestPostgresStore_RoundTrip(t *testing.T) {
	s, _ := newPGStore(t)
	ctx := t.Context()

	bare := mustNew(t, "alice@example.com")
	bare.PasswordHash = "$argon2id$placeholder"
	require.NoError(t, s.Save(ctx, bare))
	got, err := s.ByEmail(ctx, "ALICE@example.com")
	require.NoError(t, err)
	assert.Equal(t, bare.ID, got.ID)
	assert.Equal(t, bare.PublicID, got.PublicID)
	assert.Equal(t, "$argon2id$placeholder", got.PasswordHash)
	assert.Empty(t, got.IDPIssuer, "NULL idp reads back empty")
	assert.Nil(t, got.TermsAcceptedAt)

	full := mustNew(t, "bob@example.com")
	accepted := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	full.AvatarURL, full.IDPIssuer, full.IDPSubject, full.Locale, full.TermsAcceptedAt = "https://x/b.png", "https://idp", "sub-1", "id", &accepted
	require.NoError(t, s.Save(ctx, full))
	got, err = s.ByID(ctx, full.ID)
	require.NoError(t, err)
	assert.Equal(t, "https://x/b.png", got.AvatarURL)
	assert.Equal(t, "https://idp", got.IDPIssuer)
	assert.Equal(t, "sub-1", got.IDPSubject)
	assert.Equal(t, "id", got.Locale)
	require.NotNil(t, got.TermsAcceptedAt)
	assert.True(t, accepted.Equal(*got.TermsAcceptedAt))
}

func TestPostgresStore_SaveUpdates(t *testing.T) {
	s, _ := newPGStore(t)
	ctx := t.Context()
	u := mustNew(t, "alice@example.com")
	require.NoError(t, s.Save(ctx, u))
	created := u.CreatedAt

	u.Name, u.PasswordHash, u.Email = "Alice B", "$argon2id$new", "alice.b@example.com"
	u.CreatedAt = time.Now().Add(time.Hour)
	require.NoError(t, s.Save(ctx, u))

	got, err := s.ByID(ctx, u.ID)
	require.NoError(t, err)
	assert.Equal(t, "Alice B", got.Name)
	assert.Equal(t, "$argon2id$new", got.PasswordHash)
	assert.Equal(t, "alice.b@example.com", got.Email)
	assert.WithinDuration(t, created, got.CreatedAt, time.Millisecond, "created_at never changes on update")
	assert.True(t, got.UpdatedAt.After(created) || got.UpdatedAt.Equal(created))
	_, err = s.ByEmail(ctx, "alice@example.com")
	assert.True(t, user.IsNotFoundError(err), "old email no longer resolves")
}

func TestPostgresStore_Uniqueness(t *testing.T) {
	s, _ := newPGStore(t)
	ctx := t.Context()
	first := mustNew(t, "alice@example.com")
	first.IDPIssuer, first.IDPSubject = "https://idp", "sub-1"
	require.NoError(t, s.Save(ctx, first))

	for name, mutate := range map[string]func(u *user.User){
		"duplicate email (case)": func(u *user.User) {
			n := mustNew(t, "Alice@Example.com")
			*u = *n
		},
		"duplicate public id": func(u *user.User) { u.PublicID = first.PublicID },
		"duplicate idp pair":  func(u *user.User) { u.IDPIssuer, u.IDPSubject = "https://idp", "sub-1" },
	} {
		t.Run(name, func(t *testing.T) {
			u := mustNew(t, "other-"+uuid.NewString()[:8]+"@example.com")
			mutate(u)
			assert.True(t, user.IsAlreadyExistsError(s.Save(ctx, u)))
		})
	}
	t.Run("same subject at another issuer is fine", func(t *testing.T) {
		u := mustNew(t, "carol@example.com")
		u.IDPIssuer, u.IDPSubject = "https://other-idp", "sub-1"
		assert.NoError(t, s.Save(ctx, u))
	})
}

func TestPostgresStore_CheckConstraints(t *testing.T) {
	s, pool := newPGStore(t)
	ctx := t.Context()

	t.Run("email must be stored normalized", func(t *testing.T) {
		_, err := pool.W.ExecContext(ctx,
			`INSERT INTO users (id, public_id, email, created_at, updated_at) VALUES ($1, 'p1', 'Alice@Example.com', now(), now())`, uuid.New())
		assert.ErrorContains(t, err, "users_email_normalized")
	})
	t.Run("idp issuer and subject come together", func(t *testing.T) {
		u := mustNew(t, "half@example.com")
		u.IDPIssuer = "https://idp"
		err := s.Save(ctx, u)
		require.Error(t, err)
		assert.False(t, user.IsAlreadyExistsError(err))
		assert.ErrorContains(t, err, "users_idp_pair")
	})
}

func TestPostgresStore_HonorsTransaction(t *testing.T) {
	s, pool := newPGStore(t)
	ctx := t.Context()
	rolledBack := mustNew(t, "rollback@example.com")
	committed := mustNew(t, "commit@example.com")

	errAbort := errors.New("abort")
	err := db.RunInTx(ctx, pool, func(ctx context.Context) error {
		require.NoError(t, s.Save(ctx, rolledBack))
		got, err := s.ByID(ctx, rolledBack.ID)
		require.NoError(t, err, "visible inside its own transaction")
		assert.Equal(t, rolledBack.Email, got.Email)
		return errAbort
	})
	require.ErrorIs(t, err, errAbort)
	_, err = s.ByID(ctx, rolledBack.ID)
	assert.True(t, user.IsNotFoundError(err), "rolled back")

	require.NoError(t, db.RunInTx(ctx, pool, func(ctx context.Context) error { return s.Save(ctx, committed) }))
	_, err = s.ByEmail(ctx, committed.Email)
	assert.NoError(t, err)
}

func TestPostgresStore_AcceptTermsUsesTheDatabaseClock(t *testing.T) {
	s, pool := newPGStore(t)
	ctx := t.Context()
	u := mustNew(t, "ada@example.com")
	require.NoError(t, s.Save(ctx, u))

	var dbNow time.Time
	var got *user.User
	require.NoError(t, db.RunInTx(ctx, pool, func(ctx context.Context) error {
		tx, _ := db.CurrentTx(ctx)
		if err := tx.QueryRowContext(ctx, `SELECT now()`).Scan(&dbNow); err != nil {
			return err
		}
		var err error
		got, err = s.AcceptTerms(ctx, u.ID)
		return err
	}))
	require.NotNil(t, got.TermsAcceptedAt)
	assert.True(t, dbNow.Equal(*got.TermsAcceptedAt), "%v vs %v", dbNow, *got.TermsAcceptedAt)
	assert.Equal(t, u.Email, got.Email)

	_, err := s.AcceptTerms(ctx, uuid.New())
	assert.True(t, user.IsNotFoundError(err))
}

func TestPostgresStore_SetNameTouchesOnlyTheName(t *testing.T) {
	s, _ := newPGStore(t)
	ctx := t.Context()
	u := mustNew(t, "ada@example.com")
	u.PasswordHash = "stored-hash"
	require.NoError(t, s.Save(ctx, u))

	got, err := s.SetName(ctx, u.ID, "Ada Lovelace")
	require.NoError(t, err)
	assert.Equal(t, "Ada Lovelace", got.Name)
	assert.Equal(t, "stored-hash", got.PasswordHash)
	assert.Equal(t, u.Email, got.Email)
	assert.Equal(t, u.PublicID, got.PublicID)

	_, err = s.SetName(ctx, uuid.New(), "x")
	assert.True(t, user.IsNotFoundError(err))
}

func TestPostgresStore_ByPublicID(t *testing.T) {
	s, _ := newPGStore(t)
	u := mustNew(t, "pub@example.com")
	require.NoError(t, s.Save(t.Context(), u))

	got, err := s.ByPublicID(t.Context(), u.PublicID)
	require.NoError(t, err)
	assert.Equal(t, u.ID, got.ID)
	for _, bad := range []string{"", "nope", u.ID.String()} {
		_, err = s.ByPublicID(t.Context(), bad)
		assert.True(t, user.IsNotFoundError(err), "%q is not a public id", bad)
	}
}
