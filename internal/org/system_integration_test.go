//go:build integration

package org_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/testutil/pgtest"
	"altalune.id/oagent/internal/user"
)

func TestSystemOrg_LoadAndUnique(t *testing.T) {
	pool, _, schemaName := pgtest.MigratedRLS(t)
	s := org.NewStore(pool, schemaName, "")
	ctx := t.Context()

	_, err := s.System(ctx)
	assert.True(t, org.IsNotFoundError(err), "fresh install has no system org")

	plain, err := org.NewOrg("plain", "Plain", uuid.Nil)
	require.NoError(t, err)
	require.NoError(t, s.Save(as(ctx, plain), plain))
	_, err = s.System(ctx)
	assert.True(t, org.IsNotFoundError(err), "a normal org is not the system org")

	sys, err := org.NewSystemOrg("acme", "Acme", uuid.Nil)
	require.NoError(t, err)
	require.NoError(t, s.Save(as(ctx, sys), sys))
	got, err := s.System(ctx)
	require.NoError(t, err)
	assert.Equal(t, sys.ID, got.ID)
	assert.True(t, got.IsSystem)

	byID, err := s.ByID(as(ctx, plain), plain.ID)
	require.NoError(t, err)
	assert.False(t, byID.IsSystem)

	second, err := org.NewSystemOrg("other", "Other", uuid.Nil)
	require.NoError(t, err)
	assert.True(t, org.IsAlreadyExistsError(s.Save(as(ctx, second), second)), "the partial unique index allows one system org")
}

func TestSystemOrg_ConcurrentInstallsOneWins(t *testing.T) {
	pool, _, schemaName := pgtest.MigratedRLS(t)
	s := org.NewStore(pool, schemaName, "")
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
			errs[i] = s.Save(as(t.Context(), o), o)
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
	pool, _, schemaName := pgtest.MigratedRLS(t)
	s := org.NewStore(pool, schemaName, "")
	ctx := t.Context()
	sys, err := org.NewSystemOrg("acme", "Acme", uuid.Nil)
	require.NoError(t, err)
	require.NoError(t, s.Save(as(ctx, sys), sys))

	sys.IsSystem = false
	sys.Name = "Acme Renamed"
	require.NoError(t, s.Save(as(ctx, sys), sys))
	got, err := s.System(ctx)
	require.NoError(t, err)
	assert.Equal(t, "Acme Renamed", got.Name)
	assert.True(t, got.IsSystem, "is_system is set once at creation")

	plain, err := org.NewOrg("plain", "Plain", uuid.Nil)
	require.NoError(t, err)
	require.NoError(t, s.Save(as(ctx, plain), plain))
	plain.IsSystem = true
	require.NoError(t, s.Save(as(ctx, plain), plain))
	got, err = s.ByID(as(ctx, plain), plain.ID)
	require.NoError(t, err)
	assert.False(t, got.IsSystem, "an existing org cannot be promoted by Save")
}

func TestOrgCreatedBy_ForeignKey(t *testing.T) {
	pool, admin, schemaName := pgtest.MigratedRLS(t)
	orgs, users := org.NewStore(pool, schemaName, ""), user.NewStore(pool, schemaName, "")
	ctx := t.Context()

	ghost, err := org.NewOrg("ghost", "Ghost", uuid.New())
	require.NoError(t, err)
	err = orgs.Save(as(ctx, ghost), ghost)
	pgErr, ok := errors.AsType[*pgconn.PgError](err)
	require.True(t, ok, "got %v", err)
	assert.Equal(t, "23503", pgErr.Code, "created_by must name a real user")

	u, err := user.New("founder@example.com", "")
	require.NoError(t, err)
	require.NoError(t, users.Save(ctx, u))
	o, err := org.NewOrg("acme", "Acme", u.ID)
	require.NoError(t, err)
	require.NoError(t, orgs.Save(as(ctx, o), o))

	_, err = admin.ExecContext(ctx, `DELETE FROM `+pgx.Identifier{schemaName, "users"}.Sanitize()+` WHERE id = $1`, u.ID)
	require.NoError(t, err)
	got, err := orgs.ByID(as(ctx, o), o.ID)
	require.NoError(t, err, "deleting the founder keeps the org")
	assert.False(t, got.CreatedBy.Valid, "created_by is set to NULL")
}

func as(ctx context.Context, o *org.Org) context.Context {
	return tenant.Into(ctx, tenant.Context{OrgID: o.ID})
}
