//go:build integration

package modelprovider_test

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/modelprovider"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/project"
	"altalune.id/oagent/internal/testutil/pgtest"
)

type pgFixture struct {
	admin    *sql.DB
	pool     db.Pool
	store    modelprovider.Store
	orgs     org.Store
	projects project.Store
}

func newPgFixture(t *testing.T) *pgFixture {
	t.Helper()
	pool, admin, schemaName := pgtest.MigratedRLS(t)
	return &pgFixture{
		admin:    admin,
		pool:     pool,
		store:    modelprovider.NewStore(pool, schemaName, ""),
		orgs:     org.NewStore(pool, schemaName, ""),
		projects: project.NewStore(pool, schemaName, ""),
	}
}

func in(orgID uuid.UUID) context.Context {
	return tenant.Into(context.Background(), tenant.Context{OrgID: orgID})
}

func (f *pgFixture) seedScope(t *testing.T) (orgID, projectID uuid.UUID) {
	t.Helper()
	o, err := org.NewOrg("org-"+uuid.NewString()[:8], "Org", uuid.Nil)
	require.NoError(t, err)
	require.NoError(t, f.orgs.Save(in(o.ID), o))
	return o.ID, f.seedProject(t, o.ID)
}

func (f *pgFixture) seedProject(t *testing.T, orgID uuid.UUID) uuid.UUID {
	t.Helper()
	p, err := project.NewProject(orgID, "prj-"+uuid.NewString()[:8], "Project", uuid.Nil)
	require.NoError(t, err)
	require.NoError(t, f.projects.Save(in(orgID), p))
	return p.ID
}

func newProvider(t *testing.T, orgID, projectID uuid.UUID, name string, created time.Time) *modelprovider.Provider {
	t.Helper()
	p, err := modelprovider.New(orgID, projectID, modelprovider.Input{
		Name:    name,
		Kind:    modelprovider.KindOpenAICompatible,
		BaseURL: "https://llm.example.com/v1",
		Models: []modelprovider.Model{
			{ID: "gpt-4o", Vision: true, Prices: &modelprovider.Prices{InputMicros: 2_500_000, OutputMicros: 10_000_000, CacheReadMicros: 1, CacheWriteMicros: 2}},
			{ID: "llama-3"},
		},
	}, created)
	require.NoError(t, err)
	p.APIKeySealed = []byte{0xde, 0xad}
	return p
}

func pgCode(err error) string {
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		return pgErr.Code
	}
	return ""
}

func TestStore_SaveAndRead(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	ctx := in(orgID)
	base := time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC)

	first := newProvider(t, orgID, projectID, "first", base)
	second := newProvider(t, orgID, projectID, "second", base.Add(time.Minute))
	require.NoError(t, f.store.Save(ctx, second))
	require.NoError(t, f.store.Save(ctx, first))

	got, err := f.store.ByID(ctx, first.ID)
	require.NoError(t, err)
	assert.Equal(t, first, got)

	list, err := f.store.List(ctx, projectID)
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, []uuid.UUID{first.ID, second.ID}, []uuid.UUID{list[0].ID, list[1].ID}, "oldest first")

	ref, err := f.store.Resolve(context.Background(), first.PublicID)
	require.NoError(t, err)
	assert.Equal(t, modelprovider.Ref{ID: first.ID, OrgID: orgID, ProjectID: projectID}, ref)

	_, err = first.Update(modelprovider.Input{
		Name: "renamed", Kind: modelprovider.KindOpenAICompatible, BaseURL: "https://other.example.com/v1",
		Models: []modelprovider.Model{{ID: "only"}},
	}, base.Add(time.Hour))
	require.NoError(t, err)
	first.APIKeySealed = []byte{0xbe, 0xef}
	require.NoError(t, f.store.Save(ctx, first))

	got, err = f.store.ByID(ctx, first.ID)
	require.NoError(t, err)
	assert.Equal(t, first, got)
	assert.Equal(t, base, got.CreatedAt, "created_at never changes")
}

func TestStore_DuplicateNameIsAlreadyExists(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	ctx := in(orgID)

	require.NoError(t, f.store.Save(ctx, newProvider(t, orgID, projectID, "dup", time.Now())))
	err := f.store.Save(ctx, newProvider(t, orgID, projectID, "dup", time.Now()))
	assert.True(t, modelprovider.IsAlreadyExistsError(err), "got %v", err)

	otherProject := f.seedProject(t, orgID)
	assert.NoError(t, f.store.Save(ctx, newProvider(t, orgID, otherProject, "dup", time.Now())), "the name is unique per project only")
}

func TestStore_ProjectDeleteCascades(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	ctx := in(orgID)
	p := newProvider(t, orgID, projectID, "gone", time.Now())
	require.NoError(t, f.store.Save(ctx, p))

	_, err := f.admin.ExecContext(t.Context(), `DELETE FROM projects WHERE id = $1`, projectID)
	require.NoError(t, err)

	_, err = f.store.ByID(ctx, p.ID)
	assert.True(t, modelprovider.IsNotFoundError(err), "got %v", err)
}

func TestStore_CrossOrgProjectRefused(t *testing.T) {
	f := newPgFixture(t)
	orgA, _ := f.seedScope(t)
	_, projectB := f.seedScope(t)

	err := f.store.Save(in(orgA), newProvider(t, orgA, projectB, "cross", time.Now()))
	require.Error(t, err)
	assert.Equal(t, "23503", pgCode(err), "foreign key violation, got %v", err)
}

func TestStore_OtherTenantInvisible(t *testing.T) {
	f := newPgFixture(t)
	orgA, projectA := f.seedScope(t)
	orgB, projectB := f.seedScope(t)
	victim := newProvider(t, orgA, projectA, "victim", time.Now())
	require.NoError(t, f.store.Save(in(orgA), victim))

	_, err := f.store.ByID(in(orgB), victim.ID)
	assert.True(t, modelprovider.IsNotFoundError(err), "ByID: %v", err)

	list, err := f.store.List(in(orgB), projectA)
	require.NoError(t, err)
	assert.Empty(t, list)

	assert.True(t, modelprovider.IsNotFoundError(f.store.Delete(in(orgB), victim.ID)))

	assert.True(t, modelprovider.IsNotFoundError(f.store.Save(in(orgB), victim)), "a row naming another org is refused")

	intruder := newProvider(t, orgB, projectB, "hijacked", time.Now())
	intruder.ID = victim.ID
	assert.Error(t, f.store.Save(in(orgB), intruder), "an upsert onto another org's row id is refused")

	err = db.RunInTx(in(orgB), f.pool, func(ctx context.Context) error {
		_, err := f.store.LockForUpdate(ctx, victim.ID)
		return err
	})
	assert.True(t, modelprovider.IsNotFoundError(err), "LockForUpdate: %v", err)
	err = db.RunInTx(in(orgB), f.pool, func(ctx context.Context) error { return f.store.LockShared(ctx, []uuid.UUID{victim.ID}) })
	assert.True(t, modelprovider.IsNotFoundError(err), "LockShared: %v", err)

	got, err := f.store.ByID(in(orgA), victim.ID)
	require.NoError(t, err)
	assert.Equal(t, "victim", got.Name)
}

func TestStore_MissingIsNotFound(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	ctx := in(orgID)
	missing := uuid.New()

	_, err := f.store.ByID(ctx, missing)
	assert.True(t, modelprovider.IsNotFoundError(err), "ByID: %v", err)
	assert.True(t, modelprovider.IsNotFoundError(f.store.Delete(ctx, missing)), "Delete")
	_, err = f.store.Resolve(context.Background(), "nope")
	assert.True(t, modelprovider.IsNotFoundError(err), "Resolve: %v", err)

	err = db.RunInTx(ctx, f.pool, func(ctx context.Context) error {
		_, err := f.store.LockForUpdate(ctx, missing)
		return err
	})
	assert.True(t, modelprovider.IsNotFoundError(err), "LockForUpdate: %v", err)

	present := newProvider(t, orgID, projectID, "present", time.Now())
	require.NoError(t, f.store.Save(ctx, present))
	err = db.RunInTx(ctx, f.pool, func(ctx context.Context) error {
		return f.store.LockShared(ctx, []uuid.UUID{present.ID, missing})
	})
	assert.True(t, modelprovider.IsNotFoundError(err), "LockShared: %v", err)
	assert.NoError(t, db.RunInTx(ctx, f.pool, func(ctx context.Context) error { return f.store.LockShared(ctx, nil) }))
}

func TestStore_LocksNeedAUnitOfWork(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	ctx := in(orgID)
	p := newProvider(t, orgID, projectID, "p", time.Now())
	require.NoError(t, f.store.Save(ctx, p))

	_, err := f.store.LockForUpdate(ctx, p.ID)
	assert.Error(t, err)
	assert.Error(t, f.store.LockShared(ctx, []uuid.UUID{p.ID}))
}

func TestStore_LockSharedBlocksLockForUpdate(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	ctx := in(orgID)
	p := newProvider(t, orgID, projectID, "locked", time.Now())
	require.NoError(t, f.store.Save(ctx, p))

	shared := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	unlock := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(unlock)
	sharedDone := make(chan error, 1)
	go func() {
		sharedDone <- db.RunInTx(ctx, f.pool, func(ctx context.Context) error {
			if err := f.store.LockShared(ctx, []uuid.UUID{p.ID}); err != nil {
				close(shared)
				return err
			}
			close(shared)
			<-release
			return nil
		})
	}()
	<-shared

	updated := make(chan error, 1)
	go func() {
		updated <- db.RunInTx(ctx, f.pool, func(ctx context.Context) error {
			_, err := f.store.LockForUpdate(ctx, p.ID)
			return err
		})
	}()

	require.Eventually(t, func() bool {
		var waiting int
		err := f.admin.QueryRowContext(t.Context(), `SELECT count(*) FROM pg_stat_activity
			WHERE datname = current_database() AND wait_event_type = 'Lock' AND query ILIKE '%FOR UPDATE%'`).Scan(&waiting)
		return err == nil && waiting > 0
	}, 10*time.Second, 20*time.Millisecond, "LockForUpdate waits on the share lock")
	select {
	case err := <-updated:
		t.Fatalf("LockForUpdate returned while the share lock was held: %v", err)
	default:
	}

	unlock()
	require.NoError(t, <-sharedDone)
	select {
	case err := <-updated:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("LockForUpdate never got the lock after the share lock ended")
	}
}
