//go:build integration

package org_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/testutil/pgtest"
	"altalune.id/oagent/internal/user"
	"altalune.id/oagent/migrator"
	"altalune.id/oagent/schema"
)

func TestSystemOrg_LoadAndUnique(t *testing.T) {
	sqlDB, schemaName := pgtest.Migrated(t)
	s := org.NewStore(db.Pool{W: sqlDB, R: sqlDB}, schemaName, "")
	ctx := t.Context()

	_, err := s.System(ctx)
	assert.True(t, org.IsNotFoundError(err), "fresh install has no system org")

	plain, err := org.NewOrg("plain", "Plain", uuid.Nil)
	require.NoError(t, err)
	require.NoError(t, s.Save(ctx, plain))
	_, err = s.System(ctx)
	assert.True(t, org.IsNotFoundError(err), "a normal org is not the system org")

	sys, err := org.NewSystemOrg("acme", "Acme", uuid.Nil)
	require.NoError(t, err)
	require.NoError(t, s.Save(ctx, sys))
	got, err := s.System(ctx)
	require.NoError(t, err)
	assert.Equal(t, sys.ID, got.ID)
	assert.True(t, got.IsSystem)

	byID, err := s.ByID(ctx, plain.ID)
	require.NoError(t, err)
	assert.False(t, byID.IsSystem)

	second, err := org.NewSystemOrg("other", "Other", uuid.Nil)
	require.NoError(t, err)
	assert.True(t, org.IsAlreadyExistsError(s.Save(ctx, second)), "the partial unique index allows one system org")
}

func TestSystemOrg_ConcurrentInstallsOneWins(t *testing.T) {
	sqlDB, schemaName := pgtest.Migrated(t)
	s := org.NewStore(db.Pool{W: sqlDB, R: sqlDB}, schemaName, "")
	const n = 8
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			o, err := org.NewSystemOrg("sys-"+uuid.NewString()[:8], "Sys", uuid.Nil)
			if err != nil {
				errs[i] = err
				return
			}
			errs[i] = s.Save(t.Context(), o)
		})
	}
	wg.Wait()
	wins := 0
	for _, err := range errs {
		if err == nil {
			wins++
			continue
		}
		assert.True(t, org.IsAlreadyExistsError(err), "got %v", err)
	}
	assert.Equal(t, 1, wins)
}

func TestSystemOrg_FlagNeverChangesOnSave(t *testing.T) {
	sqlDB, schemaName := pgtest.Migrated(t)
	s := org.NewStore(db.Pool{W: sqlDB, R: sqlDB}, schemaName, "")
	ctx := t.Context()
	sys, err := org.NewSystemOrg("acme", "Acme", uuid.Nil)
	require.NoError(t, err)
	require.NoError(t, s.Save(ctx, sys))

	sys.IsSystem = false
	sys.Name = "Acme Renamed"
	require.NoError(t, s.Save(ctx, sys))
	got, err := s.System(ctx)
	require.NoError(t, err)
	assert.Equal(t, "Acme Renamed", got.Name)
	assert.True(t, got.IsSystem, "is_system is set once at creation")

	plain, err := org.NewOrg("plain", "Plain", uuid.Nil)
	require.NoError(t, err)
	require.NoError(t, s.Save(ctx, plain))
	plain.IsSystem = true
	require.NoError(t, s.Save(ctx, plain))
	got, err = s.ByID(ctx, plain.ID)
	require.NoError(t, err)
	assert.False(t, got.IsSystem, "an existing org cannot be promoted by Save")
}

func TestOrgCreatedBy_ForeignKey(t *testing.T) {
	sqlDB, schemaName := pgtest.Migrated(t)
	pool := db.Pool{W: sqlDB, R: sqlDB}
	orgs, users := org.NewStore(pool, schemaName, ""), user.NewStore(pool, schemaName, "")
	ctx := t.Context()

	ghost, err := org.NewOrg("ghost", "Ghost", uuid.New())
	require.NoError(t, err)
	err = orgs.Save(ctx, ghost)
	pgErr, ok := errors.AsType[*pgconn.PgError](err)
	require.True(t, ok, "got %v", err)
	assert.Equal(t, "23503", pgErr.Code, "created_by must name a real user")

	u, err := user.New("founder@example.com", "")
	require.NoError(t, err)
	require.NoError(t, users.Save(ctx, u))
	o, err := org.NewOrg("acme", "Acme", u.ID)
	require.NoError(t, err)
	require.NoError(t, orgs.Save(ctx, o))

	_, err = sqlDB.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, u.ID)
	require.NoError(t, err)
	got, err := orgs.ByID(ctx, o.ID)
	require.NoError(t, err, "deleting the founder keeps the org")
	assert.False(t, got.CreatedBy.Valid, "created_by is set to NULL")
}

func TestMigration009_ClearsOrphanCreatedBy(t *testing.T) {
	h := pgtest.New(t)
	sqlDB := h.OpenDB(t)
	cfg := migrator.Config{Schema: h.Schema}
	ctx := t.Context()
	require.NoError(t, schema.MigrateUp(ctx, sqlDB, cfg))
	require.NoError(t, schema.MigrateDownTo(ctx, sqlDB, cfg, 8))

	orphan, kept := uuid.New(), uuid.New()
	_, err := sqlDB.ExecContext(ctx,
		`INSERT INTO users (id, public_id, email, created_at, updated_at) VALUES ($1, 'u-kept', 'kept@example.com', now(), now())`, kept)
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx, `INSERT INTO orgs (id, public_id, slug, name, created_by, created_at, updated_at) VALUES
		($1, 'o-orphan', 'orphan', 'Orphan', $2, now(), now()),
		($3, 'o-kept', 'kept', 'Kept', $4, now(), now())`, uuid.New(), orphan, uuid.New(), kept)
	require.NoError(t, err)

	require.NoError(t, schema.MigrateUp(ctx, sqlDB, cfg), "009 must apply on a DB that was not reset")

	var orphanBy, keptBy *uuid.UUID
	require.NoError(t, sqlDB.QueryRowContext(ctx, `SELECT created_by FROM orgs WHERE slug = 'orphan'`).Scan(&orphanBy))
	require.NoError(t, sqlDB.QueryRowContext(ctx, `SELECT created_by FROM orgs WHERE slug = 'kept'`).Scan(&keptBy))
	assert.Nil(t, orphanBy, "an orphan created_by is cleared")
	require.NotNil(t, keptBy)
	assert.Equal(t, kept, *keptBy, "a valid created_by is kept")
}
