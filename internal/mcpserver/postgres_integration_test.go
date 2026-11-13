//go:build integration

package mcpserver_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/mcpserver"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/project"
	"altalune.id/oagent/internal/testutil/pgtest"
)

type pgFixture struct {
	admin    *sql.DB
	pool     db.Pool
	schema   string
	store    mcpserver.Store
	orgs     org.Store
	projects project.Store
}

func newPgFixture(t *testing.T) *pgFixture {
	t.Helper()
	pool, admin, schemaName := pgtest.MigratedRLS(t)
	return &pgFixture{
		admin:    admin,
		pool:     pool,
		schema:   schemaName,
		store:    mcpserver.NewStore(pool, schemaName, ""),
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

func newServer(t *testing.T, orgID, projectID uuid.UUID, name string, kind mcpserver.AuthKind, created time.Time) *mcpserver.Server {
	t.Helper()
	s, err := mcpserver.New(orgID, projectID, mcpserver.Input{
		Name: name, URL: "https://mcp.example.com/mcp", AuthKind: kind, AuthHeader: "X-Api-Key", TimeoutMS: 5000,
	}, created)
	require.NoError(t, err)
	if kind != mcpserver.AuthNone {
		s.AuthSealed = []byte{0xde, 0xad}
	}
	return s
}

func pgCode(err error) string {
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		return pgErr.Code
	}
	return ""
}

func TestStore_SaveAndReadEveryAuthKind(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	ctx := in(orgID)
	base := time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC)

	var saved []*mcpserver.Server
	for i, kind := range []mcpserver.AuthKind{mcpserver.AuthNone, mcpserver.AuthBearer, mcpserver.AuthHeader} {
		s := newServer(t, orgID, projectID, "s-"+string(kind), kind, base.Add(time.Duration(i)*time.Minute))
		require.NoError(t, f.store.Save(ctx, s))
		got, err := f.store.ByID(ctx, s.ID)
		require.NoError(t, err)
		assert.Equal(t, s, got, kind)
		saved = append(saved, s)
	}
	assert.Nil(t, saved[0].AuthSealed)
	assert.Empty(t, saved[1].AuthHeader)
	assert.Equal(t, "X-Api-Key", saved[2].AuthHeader)

	list, err := f.store.List(ctx, projectID)
	require.NoError(t, err)
	require.Len(t, list, 3)
	assert.Equal(t, []uuid.UUID{saved[0].ID, saved[1].ID, saved[2].ID}, []uuid.UUID{list[0].ID, list[1].ID, list[2].ID}, "oldest first")

	ref, err := f.store.Resolve(context.Background(), saved[1].PublicID)
	require.NoError(t, err)
	assert.Equal(t, mcpserver.Ref{ID: saved[1].ID, OrgID: orgID, ProjectID: projectID}, ref)

	s := saved[2]
	_, err = s.Update(mcpserver.Input{URL: "https://other.example.com/mcp", AuthKind: mcpserver.AuthNone, TimeoutMS: 9000, Trusted: true}, base.Add(time.Hour))
	require.NoError(t, err)
	require.NoError(t, f.store.Save(ctx, s))
	got, err := f.store.ByID(ctx, s.ID)
	require.NoError(t, err)
	assert.Equal(t, s, got)
	assert.Equal(t, base.Add(2*time.Minute), got.CreatedAt, "created_at never changes")
}

func TestStore_DuplicateNameIsAlreadyExists(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	ctx := in(orgID)
	require.NoError(t, f.store.Save(ctx, newServer(t, orgID, projectID, "dup", mcpserver.AuthNone, time.Now())))
	err := f.store.Save(ctx, newServer(t, orgID, projectID, "dup", mcpserver.AuthNone, time.Now()))
	assert.True(t, mcpserver.IsAlreadyExistsError(err), "got %v", err)
	assert.NoError(t, f.store.Save(ctx, newServer(t, orgID, f.seedProject(t, orgID), "dup", mcpserver.AuthNone, time.Now())))
}

func TestStore_ChecksRefuseBadRows(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	for name, row := range map[string]struct {
		name, kind string
		header     any
		sealed     any
		timeout    int
		constraint string
	}{
		"name with an underscore": {"my_server", "none", nil, nil, 5000, "mcp_servers_name_format"},
		"none with a secret":      {"a", "none", nil, []byte{1}, 5000, "mcp_servers_auth_sealed_shape"},
		"bearer without a secret": {"b", "bearer", nil, nil, 5000, "mcp_servers_auth_sealed_shape"},
		"header without a name":   {"c", "header", nil, []byte{1}, 5000, "mcp_servers_auth_header_shape"},
		"bearer with a header":    {"d", "bearer", "X-Key", []byte{1}, 5000, "mcp_servers_auth_header_shape"},
		"unknown auth kind":       {"e", "basic", nil, []byte{1}, 5000, "mcp_servers_auth_kind_check"},
		"timeout below the range": {"f", "none", nil, nil, mcpserver.MinTimeoutMS - 1, "mcp_servers_timeout_range"},
		"timeout above the range": {"g", "none", nil, nil, mcpserver.MaxTimeoutMS + 1, "mcp_servers_timeout_range"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := f.admin.ExecContext(t.Context(), `INSERT INTO `+f.schema+`.mcp_servers
				(id, public_id, org_id, project_id, name, url, auth_kind, auth_header, auth_sealed, timeout_ms, trusted, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, 'https://e.com/mcp', $6, $7, $8, $9, false, now(), now())`,
				uuid.New(), uuid.NewString(), orgID, projectID, row.name, row.kind, row.header, row.sealed, row.timeout)
			require.Error(t, err)
			assert.Equal(t, "23514", pgCode(err), "check violation: %v", err)
			pgErr, _ := errors.AsType[*pgconn.PgError](err)
			require.NotNil(t, pgErr)
			assert.Equal(t, row.constraint, pgErr.ConstraintName)
		})
	}
}

// SECURITY: the timeout CHECK takes exactly the domain range, so a row the domain accepts is never refused and the reverse.
func TestStore_TimeoutCheckMatchesTheDomainRange(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	for i, ms := range []int{mcpserver.MinTimeoutMS, mcpserver.RecommendedTimeoutMS, mcpserver.MaxTimeoutMS} {
		_, err := f.admin.ExecContext(t.Context(), `INSERT INTO `+f.schema+`.mcp_servers
			(id, public_id, org_id, project_id, name, url, auth_kind, auth_header, auth_sealed, timeout_ms, trusted, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, 'https://e.com/mcp', 'none', NULL, NULL, $6, false, now(), now())`,
			uuid.New(), uuid.NewString(), orgID, projectID, fmt.Sprintf("edge%d", i), ms)
		assert.NoError(t, err, "%d", ms)
	}
}

func TestStore_ProjectDeleteCascades(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	s := newServer(t, orgID, projectID, "gone", mcpserver.AuthNone, time.Now())
	require.NoError(t, f.store.Save(in(orgID), s))
	_, err := f.admin.ExecContext(t.Context(), `DELETE FROM `+f.schema+`.projects WHERE id = $1`, projectID)
	require.NoError(t, err)
	_, err = f.store.ByID(in(orgID), s.ID)
	assert.True(t, mcpserver.IsNotFoundError(err), "got %v", err)
}

func TestStore_CrossOrgProjectRefused(t *testing.T) {
	f := newPgFixture(t)
	orgA, _ := f.seedScope(t)
	_, projectB := f.seedScope(t)
	err := f.store.Save(in(orgA), newServer(t, orgA, projectB, "cross", mcpserver.AuthNone, time.Now()))
	require.Error(t, err)
	assert.Equal(t, "23503", pgCode(err), "foreign key violation, got %v", err)
}

func TestStore_OtherTenantInvisible(t *testing.T) {
	f := newPgFixture(t)
	orgA, projectA := f.seedScope(t)
	orgB, projectB := f.seedScope(t)
	victim := newServer(t, orgA, projectA, "victim", mcpserver.AuthBearer, time.Now())
	require.NoError(t, f.store.Save(in(orgA), victim))

	_, err := f.store.ByID(in(orgB), victim.ID)
	assert.True(t, mcpserver.IsNotFoundError(err), "ByID: %v", err)
	list, err := f.store.List(in(orgB), projectA)
	require.NoError(t, err)
	assert.Empty(t, list)
	assert.True(t, mcpserver.IsNotFoundError(f.store.Delete(in(orgB), victim.ID)))
	assert.True(t, mcpserver.IsNotFoundError(f.store.Save(in(orgB), victim)), "a row naming another org is refused")

	intruder := newServer(t, orgB, projectB, "hijacked", mcpserver.AuthNone, time.Now())
	intruder.ID = victim.ID
	assert.True(t, mcpserver.IsNotFoundError(f.store.Save(in(orgB), intruder)), "an upsert onto another org's row id answers like an unknown one")

	err = db.RunInTx(in(orgB), f.pool, func(ctx context.Context) error {
		_, err := f.store.LockForUpdate(ctx, victim.ID)
		return err
	})
	assert.True(t, mcpserver.IsNotFoundError(err), "LockForUpdate: %v", err)
	err = db.RunInTx(in(orgB), f.pool, func(ctx context.Context) error { return f.store.LockShared(ctx, []uuid.UUID{victim.ID}) })
	assert.True(t, mcpserver.IsNotFoundError(err), "LockShared: %v", err)

	got, err := f.store.ByID(in(orgA), victim.ID)
	require.NoError(t, err)
	assert.Equal(t, "victim", got.Name)
	assert.Equal(t, mcpserver.AuthBearer, got.AuthKind)
}

func TestStore_MissingIsNotFound(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	ctx := in(orgID)
	missing := uuid.New()

	_, err := f.store.ByID(ctx, missing)
	assert.True(t, mcpserver.IsNotFoundError(err), "ByID: %v", err)
	assert.True(t, mcpserver.IsNotFoundError(f.store.Delete(ctx, missing)), "Delete")
	_, err = f.store.Resolve(context.Background(), "nope")
	assert.True(t, mcpserver.IsNotFoundError(err), "Resolve: %v", err)
	err = db.RunInTx(ctx, f.pool, func(ctx context.Context) error {
		_, err := f.store.LockForUpdate(ctx, missing)
		return err
	})
	assert.True(t, mcpserver.IsNotFoundError(err), "LockForUpdate: %v", err)

	present := newServer(t, orgID, projectID, "present", mcpserver.AuthNone, time.Now())
	require.NoError(t, f.store.Save(ctx, present))
	err = db.RunInTx(ctx, f.pool, func(ctx context.Context) error { return f.store.LockShared(ctx, []uuid.UUID{present.ID, missing}) })
	assert.True(t, mcpserver.IsNotFoundError(err), "LockShared: %v", err)
	assert.NoError(t, db.RunInTx(ctx, f.pool, func(ctx context.Context) error { return f.store.LockShared(ctx, nil) }))
}

func TestStore_LocksNeedAUnitOfWork(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	ctx := in(orgID)
	s := newServer(t, orgID, projectID, "s", mcpserver.AuthNone, time.Now())
	require.NoError(t, f.store.Save(ctx, s))
	_, err := f.store.LockForUpdate(ctx, s.ID)
	assert.Error(t, err)
	assert.Error(t, f.store.LockShared(ctx, []uuid.UUID{s.ID}))
}

func TestStore_LockSharedBlocksLockForUpdate(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	ctx := in(orgID)
	s := newServer(t, orgID, projectID, "locked", mcpserver.AuthNone, time.Now())
	require.NoError(t, f.store.Save(ctx, s))

	shared := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	unlock := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(unlock)
	sharedDone := make(chan error, 1)
	go func() {
		sharedDone <- db.RunInTx(ctx, f.pool, func(ctx context.Context) error {
			err := f.store.LockShared(ctx, []uuid.UUID{s.ID})
			close(shared)
			if err != nil {
				return err
			}
			<-release
			return nil
		})
	}()
	<-shared

	updated := make(chan error, 1)
	go func() {
		updated <- db.RunInTx(ctx, f.pool, func(ctx context.Context) error {
			_, err := f.store.LockForUpdate(ctx, s.ID)
			return err
		})
	}()

	require.Eventually(t, func() bool {
		var waiting int
		err := f.admin.QueryRowContext(t.Context(), `SELECT count(*) FROM pg_stat_activity
			WHERE datname = current_database() AND wait_event_type = 'Lock' AND query ILIKE '%mcp_servers%FOR UPDATE%'`).Scan(&waiting)
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
