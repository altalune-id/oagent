//go:build integration

package memory_test

import (
	"context"
	"database/sql"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/memory"
	"altalune.id/oagent/internal/memory/memorytest"
	"altalune.id/oagent/internal/memory/native"
	"altalune.id/oagent/internal/testutil/pgtest"
)

type storeFixture struct {
	pool       db.Pool
	admin      *sql.DB
	schema     string
	store      memory.Store
	orgA, orgB uuid.UUID
	prjA, prjB uuid.UUID
	other      uuid.UUID
}

func newStoreFixture(t *testing.T) storeFixture {
	t.Helper()
	pool, admin, schemaName := pgtest.MigratedRLS(t)
	f := storeFixture{pool: pool, admin: admin, schema: schemaName, store: memory.NewPostgresStore(pool, schemaName, ""), orgA: uuid.New(), orgB: uuid.New(), prjA: uuid.New(), prjB: uuid.New(), other: uuid.New()}
	exec := func(q string, args ...any) {
		t.Helper()
		_, err := admin.ExecContext(t.Context(), q, args...)
		require.NoError(t, err)
	}
	for _, o := range []uuid.UUID{f.orgA, f.orgB} {
		exec(`INSERT INTO orgs (id, public_id, slug, name, created_at, updated_at) VALUES ($1, $2, $2, 'Org', now(), now())`, o, "o"+o.String()[:8])
	}
	for prj, org := range map[uuid.UUID]uuid.UUID{f.prjA: f.orgA, f.other: f.orgA, f.prjB: f.orgB} {
		exec(`INSERT INTO projects (id, public_id, org_id, slug, name, created_at, updated_at) VALUES ($1, $2, $3, $2, 'P', now(), now())`, prj, "p"+prj.String()[:8], org)
	}
	return f
}

func (f storeFixture) exec(t *testing.T, q string, args ...any) {
	t.Helper()
	_, err := f.admin.ExecContext(t.Context(), q, args...)
	require.NoError(t, err)
}

func in(org uuid.UUID) context.Context {
	return tenant.Into(context.Background(), tenant.Context{OrgID: org})
}

func newStore(t *testing.T, org, prj uuid.UUID, name string, at time.Time) *memory.MemoryStore {
	t.Helper()
	s, err := memory.NewStore(org, prj, memory.StoreInput{Name: name, Backend: memory.BackendNative}, nativeOn, at)
	require.NoError(t, err)
	return s
}

func TestPostgresStore_InsertListGetResolveDelete(t *testing.T) {
	f := newStoreFixture(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	first, second := newStore(t, f.orgA, f.prjA, "People", now), newStore(t, f.orgA, f.prjA, "Leads", now.Add(time.Second))
	second.RememberAPIUsers = true
	require.NoError(t, f.store.InsertStore(in(f.orgA), second))
	require.NoError(t, f.store.InsertStore(in(f.orgA), first))
	require.NoError(t, f.store.InsertStore(in(f.orgA), newStore(t, f.orgA, f.other, "People", now)), "a name is unique per project only")

	list, err := f.store.ListStores(in(f.orgA), f.prjA)
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, []string{"People", "Leads"}, []string{list[0].Name, list[1].Name}, "oldest first")
	assert.Equal(t, first, list[0])
	assert.Equal(t, second, list[1], "the API-user opt-in round-trips")

	got, err := f.store.StoreByID(in(f.orgA), first.ID)
	require.NoError(t, err)
	assert.Equal(t, first, got)

	ref, err := f.store.ResolveStore(context.Background(), first.PublicID)
	require.NoError(t, err)
	assert.Equal(t, memory.Ref{ID: first.ID, OrgID: f.orgA, ProjectID: f.prjA}, ref, "resolving needs no tenant")
	_, err = f.store.ResolveStore(context.Background(), "unknwnunknwnunknwnabc")
	assert.True(t, memory.IsStoreNotFoundError(err))

	require.NoError(t, f.store.DeleteStore(in(f.orgA), first.ID))
	_, err = f.store.StoreByID(in(f.orgA), first.ID)
	assert.True(t, memory.IsStoreNotFoundError(err))
}

func TestPostgresStore_ANameTakenInTheProjectIsRefused(t *testing.T) {
	f := newStoreFixture(t)
	require.NoError(t, f.store.InsertStore(in(f.orgA), newStore(t, f.orgA, f.prjA, "People", time.Now())))
	err := f.store.InsertStore(in(f.orgA), newStore(t, f.orgA, f.prjA, "People", time.Now()))
	var taken *memory.StoreAlreadyExistsError
	require.ErrorAs(t, err, &taken)
	assert.Equal(t, "People", taken.Name)
}

func TestPostgresStore_AnotherOrgSeesAndTouchesNothing(t *testing.T) {
	f := newStoreFixture(t)
	mine := newStore(t, f.orgA, f.prjA, "People", time.Now())
	require.NoError(t, f.store.InsertStore(in(f.orgA), mine))

	assert.True(t, memory.IsStoreNotFoundError(f.store.InsertStore(in(f.orgB), newStore(t, f.orgA, f.prjA, "Leads", time.Now()))),
		"a row naming another org never lands")
	_, err := f.store.StoreByID(in(f.orgB), mine.ID)
	assert.True(t, memory.IsStoreNotFoundError(err))
	list, err := f.store.ListStores(in(f.orgB), f.prjA)
	require.NoError(t, err)
	assert.Empty(t, list)
	require.NoError(t, f.store.DeleteStore(in(f.orgB), mine.ID))
	got, err := f.store.StoreByID(in(f.orgA), mine.ID)
	require.NoError(t, err, "another org's delete leaves the store")
	assert.Equal(t, mine.ID, got.ID)
}

func TestPostgresStore_NoTenantIsRefused(t *testing.T) {
	f := newStoreFixture(t)
	ctx := context.Background()
	s := newStore(t, f.orgA, f.prjA, "People", time.Now())
	require.Error(t, f.store.InsertStore(ctx, s))
	_, err := f.store.StoreByID(ctx, s.ID)
	require.Error(t, err)
	_, err = f.store.ListStores(ctx, f.prjA)
	require.Error(t, err)
	require.Error(t, f.store.DeleteStore(ctx, s.ID))
}

func TestPostgresStore_LockStoreSharedNeedsAUnitOfWork(t *testing.T) {
	pool, admin, schemaName := pgtest.MigratedRLS(t)
	f := storeFixture{store: memory.NewPostgresStore(pool, schemaName, ""), orgA: uuid.New(), prjA: uuid.New()}
	_, err := admin.ExecContext(t.Context(), `INSERT INTO orgs (id, public_id, slug, name, created_at, updated_at) VALUES ($1, 'o1', 'o1', 'Org', now(), now())`, f.orgA)
	require.NoError(t, err)
	_, err = admin.ExecContext(t.Context(), `INSERT INTO projects (id, public_id, org_id, slug, name, created_at, updated_at) VALUES ($1, 'p1', $2, 'p', 'P', now(), now())`, f.prjA, f.orgA)
	require.NoError(t, err)
	s := newStore(t, f.orgA, f.prjA, "People", time.Now())
	require.NoError(t, f.store.InsertStore(in(f.orgA), s))

	require.Error(t, f.store.LockStoreShared(in(f.orgA), s.ID), "a share lock outside a unit of work is a bug")
	uow := db.NewTransactor(pool)
	require.NoError(t, uow.RunInTx(in(f.orgA), func(ctx context.Context) error { return f.store.LockStoreShared(ctx, s.ID) }))
	err = uow.RunInTx(in(f.orgA), func(ctx context.Context) error { return f.store.LockStoreShared(ctx, uuid.New()) })
	assert.True(t, memory.IsStoreNotFoundError(err))
	err = uow.RunInTx(in(uuid.New()), func(ctx context.Context) error { return f.store.LockStoreShared(ctx, s.ID) })
	assert.True(t, memory.IsStoreNotFoundError(err), "another org's store is not found")
	require.Error(t, f.store.LockStoreShared(context.Background(), s.ID))
}

func TestPostgresStore_LockStore(t *testing.T) {
	f := newStoreFixture(t)
	pool := f.pool
	s := newStore(t, f.orgA, f.prjA, "People", time.Now())
	require.NoError(t, f.store.InsertStore(in(f.orgA), s))
	_, err := f.store.LockStore(in(f.orgA), s.ID)
	require.Error(t, err, "a row lock outside a unit of work is a bug")
	uow := db.NewTransactor(pool)
	require.NoError(t, uow.RunInTx(in(f.orgA), func(ctx context.Context) error {
		got, err := f.store.LockStore(ctx, s.ID)
		if err == nil {
			assert.Equal(t, s.ID, got.ID)
			assert.Equal(t, memory.BackendNative, got.Backend)
		}
		return err
	}))
	err = uow.RunInTx(in(f.orgB), func(ctx context.Context) error { _, err := f.store.LockStore(ctx, s.ID); return err })
	assert.True(t, memory.IsStoreNotFoundError(err), "another org's store is not found")
	_, err = f.store.LockStore(context.Background(), s.ID)
	require.Error(t, err)
}

type publishedFlag struct{ inUse atomic.Bool }

func (p *publishedFlag) StoreInUse(context.Context, uuid.UUID) (bool, error) {
	return p.inUse.Load(), nil
}

func (p *publishedFlag) StoresUsedBy(context.Context, uuid.UUID) (map[uuid.UUID][]memory.UsedBy, error) {
	return nil, nil
}

func TestDeleteStore_WaitsBehindAPublishAndThenRefuses(t *testing.T) {
	f := newStoreFixture(t)
	s := newStore(t, f.orgA, f.prjA, "People", time.Now())
	require.NoError(t, f.store.InsertStore(in(f.orgA), s))
	uow := db.NewTransactor(f.pool)
	published := &publishedFlag{}
	svc := memory.NewService(f.store, map[memory.Backend]memory.Facts{memory.BackendNative: native.New(f.pool, f.schema, "")},
		nativeOn, memory.Deps{UOW: uow}, apperror.NewReporter(nil, false).Unexpected).WithBindings(published)

	locked, release, publishDone := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		publishDone <- uow.RunInTx(in(f.orgA), func(ctx context.Context) error {
			if err := svc.LockStoreShared(ctx, s.ID); err != nil {
				close(locked)
				return err
			}
			close(locked)
			<-release
			published.inUse.Store(true)
			return nil
		})
	}()
	<-locked
	deleted := make(chan error, 1)
	go func() { deleted <- svc.DeleteStore(in(f.orgA), s.ID) }()
	select {
	case err := <-deleted:
		t.Fatalf("the delete ran while a publish held the store: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	close(release)
	require.NoError(t, <-publishDone)
	assert.True(t, memory.IsStoreInUseError(<-deleted), "the delete sees the version the publish committed")
	_, err := f.store.StoreByID(in(f.orgA), s.ID)
	require.NoError(t, err, "and the store stays")
}

func TestDeleteStore_RemovesTheStoreAndItsFacts(t *testing.T) {
	f := newStoreFixture(t)
	s := newStore(t, f.orgA, f.prjA, "People", time.Now())
	require.NoError(t, f.store.InsertStore(in(f.orgA), s))
	agentID, userID := uuid.New(), uuid.New()
	f.exec(t, `INSERT INTO agents (id, public_id, org_id, project_id, slug, name, created_at, updated_at) VALUES ($1, 'a1', $2, $3, 'a', 'A', now(), now())`, agentID, f.orgA, f.prjA)
	f.exec(t, `INSERT INTO end_users (id, public_id, org_id, project_id, created_at, updated_at) VALUES ($1, 'e1', $2, $3, now(), now())`, userID, f.orgA, f.prjA)
	facts := native.New(f.pool, f.schema, "")
	scope := memory.Scope{OrgID: f.orgA, ProjectID: f.prjA, StoreID: s.ID, AgentID: agentID, EndUserID: userID}
	require.NoError(t, facts.Record(in(f.orgA), scope, memory.Turn{Facts: []string{"Lives in Bandung."}}))
	svc := memory.NewService(f.store, map[memory.Backend]memory.Facts{memory.BackendNative: facts}, nativeOn,
		memory.Deps{UOW: db.NewTransactor(f.pool)}, apperror.NewReporter(nil, false).Unexpected).WithBindings(&publishedFlag{})
	require.NoError(t, svc.DeleteStore(in(f.orgA), s.ID))
	_, err := f.store.StoreByID(in(f.orgA), s.ID)
	assert.True(t, memory.IsStoreNotFoundError(err))
	var n int
	require.NoError(t, f.admin.QueryRowContext(t.Context(), `SELECT count(*) FROM memory_facts WHERE store_id = $1`, s.ID).Scan(&n))
	assert.Zero(t, n)
}

type gatedFacts struct {
	memory.Facts
	cleared, release chan struct{}
}

func (g gatedFacts) ClearStore(ctx context.Context, scope memory.Scope) (int, error) {
	n, err := g.Facts.ClearStore(ctx, scope)
	close(g.cleared)
	<-g.release
	return n, err
}

func TestRecord_RacingAStoreDeleteLeavesNoFact(t *testing.T) {
	f := newStoreFixture(t)
	s := newStore(t, f.orgA, f.prjA, "People", time.Now())
	require.NoError(t, f.store.InsertStore(in(f.orgA), s))
	fake := memorytest.NewFake()
	gate := gatedFacts{Facts: fake, cleared: make(chan struct{}), release: make(chan struct{})}
	svc := memory.NewService(f.store, map[memory.Backend]memory.Facts{memory.BackendNative: gate}, nativeOn,
		memory.Deps{UOW: db.NewTransactor(f.pool)}, apperror.NewReporter(nil, false).Unexpected).WithBindings(&publishedFlag{})
	scope := memory.Scope{OrgID: f.orgA, ProjectID: f.prjA, StoreID: s.ID, AgentID: uuid.New(), EndUserID: uuid.New()}

	var once sync.Once
	release := func() { once.Do(func() { close(gate.release) }) }
	t.Cleanup(release)
	deleted := make(chan error, 1)
	go func() { deleted <- svc.DeleteStore(in(f.orgA), s.ID) }()
	<-gate.cleared
	recorded := make(chan error, 1)
	go func() { recorded <- svc.Record(in(f.orgA), scope, memory.Turn{Facts: []string{"Lives in Bandung."}}) }()
	var early error
	got := false
	select {
	case early = <-recorded:
		got = true
		t.Errorf("a record ran between the store's erase and its delete: %v", early)
	case <-time.After(300 * time.Millisecond):
	}
	release()
	require.NoError(t, <-deleted)
	if !got {
		early = <-recorded
	}
	assert.True(t, memory.IsStoreNotFoundError(early), "the record waits for the delete, then finds no store")
	n, err := fake.ClearStore(in(f.orgA), memory.Scope{OrgID: f.orgA, ProjectID: f.prjA, StoreID: s.ID})
	require.NoError(t, err)
	assert.Zero(t, n, "no fact outlives its store, whatever the driver")
}
