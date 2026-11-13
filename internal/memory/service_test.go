package memory_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/runtime"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/memory"
	"altalune.id/oagent/internal/memory/memorytest"
	"altalune.id/oagent/internal/testutil/fakes"
	"altalune.id/oagent/nanoid"
)

type refs struct {
	byPub map[string]memory.Ref
	pubs  map[uuid.UUID]string
	err   error
}

func (r *refs) Resolve(_ context.Context, publicID string) (memory.Ref, bool, error) {
	ref, ok := r.byPub[publicID]
	return ref, ok, r.err
}

func (r *refs) PublicIDs(_ context.Context, _ uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]string, error) {
	out := map[uuid.UUID]string{}
	for _, id := range ids {
		if p, ok := r.pubs[id]; ok {
			out[id] = p
		}
	}
	return out, r.err
}

type stores struct {
	byPub  map[string]memory.Ref
	byID   map[uuid.UUID]*memory.MemoryStore
	locked []uuid.UUID
	err    error
}

func (s *stores) LockStoreShared(_ context.Context, id uuid.UUID) error {
	if _, ok := s.byID[id]; !ok {
		return &memory.StoreNotFoundError{}
	}
	s.locked = append(s.locked, id)
	return s.err
}

func (s *stores) add(ref memory.Ref, backend memory.Backend) string {
	pub := nanoid.MustNew(21)
	s.byPub[pub] = ref
	s.byID[ref.ID] = &memory.MemoryStore{ID: ref.ID, PublicID: pub, OrgID: ref.OrgID, ProjectID: ref.ProjectID, Name: pub, Backend: backend}
	return pub
}

func (s *stores) InsertStore(_ context.Context, st *memory.MemoryStore) error {
	if s.err != nil {
		return s.err
	}
	for _, o := range s.byID {
		if o.ProjectID == st.ProjectID && o.Name == st.Name {
			return &memory.StoreAlreadyExistsError{Name: st.Name}
		}
	}
	s.byID[st.ID] = st
	s.byPub[st.PublicID] = memory.Ref{ID: st.ID, OrgID: st.OrgID, ProjectID: st.ProjectID}
	return nil
}

func (s *stores) LockStore(ctx context.Context, id uuid.UUID) (*memory.MemoryStore, error) {
	if !fakes.InTx(ctx) {
		return nil, errors.New("a row lock needs a unit of work")
	}
	return s.StoreByID(ctx, id)
}

func (s *stores) StoreByID(_ context.Context, id uuid.UUID) (*memory.MemoryStore, error) {
	if st, ok := s.byID[id]; ok {
		return st, s.err
	}
	return nil, &memory.StoreNotFoundError{}
}

func (s *stores) ListStores(_ context.Context, projectID uuid.UUID) ([]*memory.MemoryStore, error) {
	var out []*memory.MemoryStore
	for _, st := range s.byID {
		if st.ProjectID == projectID {
			out = append(out, st)
		}
	}
	slices.SortFunc(out, func(a, b *memory.MemoryStore) int { return strings.Compare(a.Name, b.Name) })
	return out, s.err
}

func (s *stores) ResolveStore(_ context.Context, publicID string) (memory.Ref, error) {
	if s.err != nil {
		return memory.Ref{}, s.err
	}
	ref, ok := s.byPub[publicID]
	if !ok {
		return memory.Ref{}, &memory.StoreNotFoundError{}
	}
	return ref, nil
}

func (s *stores) DeleteStore(_ context.Context, id uuid.UUID) error {
	if st, ok := s.byID[id]; ok {
		delete(s.byPub, st.PublicID)
		delete(s.byID, id)
	}
	return s.err
}

type bindings struct {
	inUse map[uuid.UUID]bool
	used  map[uuid.UUID][]memory.UsedBy
	err   error
}

func (b *bindings) StoreInUse(_ context.Context, id uuid.UUID) (bool, error) {
	return b.inUse[id], b.err
}

func (b *bindings) StoresUsedBy(context.Context, uuid.UUID) (map[uuid.UUID][]memory.UsedBy, error) {
	return b.used, b.err
}

func refOf(c memorytest.Cell) memory.Ref {
	return memory.Ref{ID: c.Scope.StoreID, OrgID: c.Scope.OrgID, ProjectID: c.Scope.ProjectID}
}

type svcFixture struct {
	store          *memorytest.Fake
	stores         *stores
	storeA, storeB string
	farStore       string
	offStore       string
	refs           *fakes.PublicIDs
	orgPub, prjPub string
	farPrjPub      string
	users, agents  *refs
	runs           *refs
	svc            *memory.Service
	cells          []memorytest.Cell
	person, agentA string
	otherPerson    string
	farPerson      string
	nobody         string
	agentB         string
	farAgent       string
	runPub         string
	run            uuid.UUID
	reported       int
}

func newSvc(t *testing.T) *svcFixture {
	t.Helper()
	id := func() string { return nanoid.MustNew(21) }
	f := &svcFixture{store: memorytest.NewFake(), person: id(), agentA: id(), otherPerson: id(), farPerson: id(), nobody: id(), agentB: id(), farAgent: id(), runPub: id()}
	f.cells = f.store.Matrix(t)
	c := f.cells[0]
	f.stores = &stores{byPub: map[string]memory.Ref{}, byID: map[uuid.UUID]*memory.MemoryStore{}}
	f.storeA = f.stores.add(refOf(c), memory.BackendNative)
	f.storeB = f.stores.add(refOf(f.cells[4]), memory.BackendNative)
	f.farStore = f.stores.add(refOf(f.cells[8]), memory.BackendNative)
	f.offStore = f.stores.add(memory.Ref{ID: uuid.New(), OrgID: c.Scope.OrgID, ProjectID: c.Scope.ProjectID}, "mem0")
	f.refs = fakes.NewPublicIDs()
	f.orgPub = f.refs.Org(c.Scope.OrgID)
	f.prjPub = f.refs.Project(c.Scope.ProjectID, c.Scope.OrgID)
	f.farPrjPub = f.refs.Project(f.cells[16].Scope.ProjectID, f.cells[16].Scope.OrgID)
	f.refs.Org(f.cells[16].Scope.OrgID)
	f.users = &refs{byPub: map[string]memory.Ref{
		f.person:      {ID: c.Scope.EndUserID, OrgID: c.Scope.OrgID, ProjectID: c.Scope.ProjectID},
		f.otherPerson: {ID: f.cells[1].Scope.EndUserID, OrgID: c.Scope.OrgID, ProjectID: c.Scope.ProjectID},
		f.farPerson:   {ID: f.cells[8].Scope.EndUserID, OrgID: f.cells[8].Scope.OrgID, ProjectID: f.cells[8].Scope.ProjectID},
	}}
	f.agents = &refs{
		byPub: map[string]memory.Ref{
			f.agentA:   {ID: c.Scope.AgentID, OrgID: c.Scope.OrgID, ProjectID: c.Scope.ProjectID},
			f.farAgent: {ID: f.cells[8].Scope.AgentID, OrgID: f.cells[8].Scope.OrgID, ProjectID: f.cells[8].Scope.ProjectID},
		},
		pubs: map[uuid.UUID]string{c.Scope.AgentID: f.agentA, f.cells[2].Scope.AgentID: f.agentB},
	}
	f.run = uuid.New()
	f.runs = &refs{pubs: map[uuid.UUID]string{f.run: f.runPub}}
	f.svc = memory.NewService(f.stores, map[memory.Backend]memory.Facts{memory.BackendNative: f.store}, nativeOn, memory.Deps{
		UOW: fakes.Transactor{}, Users: f.users, Agents: f.agents, Runs: f.runs,
	}, func(ctx context.Context, msg string, err error, _ ...any) *apperror.AppError {
		f.reported++
		return apperror.NewReporter(nil, false).Unexpected(ctx, msg, err)
	})
	for _, cell := range []memorytest.Cell{c, f.cells[2], f.cells[4]} {
		require.NoError(t, f.store.Record(cell.Ctx, cell.Scope, memory.Turn{RunID: f.run, Facts: []string{"fact of " + cell.Scope.AgentID.String()}}))
	}
	return f
}

func admin(memorytest.Cell) context.Context {
	return session.PrincipalInto(context.Background(), session.Principal{Operator: true})
}

func TestService_ListNamesAgentsAndRunsByPublicID(t *testing.T) {
	f := newSvc(t)
	page, err := f.svc.List(admin(f.cells[0]), f.storeA, f.person, "", "", 50)
	require.NoError(t, err)
	assert.Equal(t, 2, page.Total, "every agent's facts of the person")
	require.Len(t, page.Memories, 2)
	assert.ElementsMatch(t, []string{f.agentA, f.agentB}, []string{page.Memories[0].AgentID, page.Memories[1].AgentID})
	assert.Equal(t, f.runPub, page.Memories[0].SourceRunID)

	one, err := f.svc.List(admin(f.cells[0]), f.storeA, f.person, f.agentA, "", 50)
	require.NoError(t, err)
	assert.Equal(t, 1, one.Total, "the agent filter narrows the list")
}

func TestService_AGoneRunShowsNoSourceRun(t *testing.T) {
	f := newSvc(t)
	f.runs.pubs = nil
	page, err := f.svc.List(admin(f.cells[0]), f.storeA, f.person, "", "", 50)
	require.NoError(t, err)
	assert.Empty(t, page.Memories[0].SourceRunID)
}

func TestDeleteMemory_OtherPersonIsNotFound(t *testing.T) {
	f := newSvc(t)
	page, err := f.svc.List(admin(f.cells[0]), f.storeA, f.person, "", "", 50)
	require.NoError(t, err)
	id := page.Memories[0].ID
	assert.True(t, memory.IsNotFoundError(f.svc.Delete(admin(f.cells[0]), f.storeA, f.otherPerson, id)), "another person's fact is not found")
	require.NoError(t, f.svc.Delete(admin(f.cells[0]), f.storeA, f.person, id))
	assert.True(t, memory.IsNotFoundError(f.svc.Delete(admin(f.cells[0]), f.storeA, f.person, id)), "a deleted fact is not found")
}

func TestDeleteMemory_OtherProjectOrOrgIsNotFound(t *testing.T) {
	f := newSvc(t)
	page, err := f.svc.List(admin(f.cells[0]), f.storeA, f.person, "", "", 50)
	require.NoError(t, err)
	assert.True(t, memory.IsNotFoundError(f.svc.Delete(admin(f.cells[0]), f.farStore, f.farPerson, page.Memories[0].ID)))
	assert.True(t, memory.IsNotFoundError(f.svc.Delete(admin(f.cells[0]), f.storeA, f.nobody, page.Memories[0].ID)), "an unknown person reads as not found")
}

func TestService_AnAgentOfAnotherProjectIsNotFound(t *testing.T) {
	f := newSvc(t)
	_, err := f.svc.List(admin(f.cells[0]), f.storeA, f.person, f.farAgent, "", 50)
	assert.True(t, memory.IsNotFoundError(err))
	_, err = f.svc.Clear(admin(f.cells[0]), f.storeA, f.person, f.farAgent)
	assert.True(t, memory.IsNotFoundError(err))
}

func TestService_OutOfReachIsNotFound(t *testing.T) {
	f := newSvc(t)
	stranger := session.PrincipalInto(context.Background(), session.Principal{ActiveOrgID: uuid.New(), UserID: uuid.New()})
	_, err := f.svc.List(stranger, f.storeA, f.person, "", "", 50)
	assert.True(t, memory.IsNotFoundError(err), "a caller of another org learns nothing")
}

func TestClearMemories_ReturnsTheCount(t *testing.T) {
	f := newSvc(t)
	n, err := f.svc.Clear(admin(f.cells[0]), f.storeA, f.person, f.agentA)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	n, err = f.svc.Clear(admin(f.cells[0]), f.storeA, f.person, "")
	require.NoError(t, err)
	assert.Equal(t, 1, n, "the rest of the person's facts")
}

func TestService_PortFailureIsReported(t *testing.T) {
	f := newSvc(t)
	f.users.err = errors.New("db down")
	_, err := f.svc.List(admin(f.cells[0]), f.storeA, f.person, "", "", 50)
	require.Error(t, err)
	assert.Equal(t, 1, f.reported)
}

func TestService_TheStoreBoundsEveryCall(t *testing.T) {
	f := newSvc(t)
	page, err := f.svc.List(admin(f.cells[0]), f.storeB, f.person, "", "", 50)
	require.NoError(t, err)
	require.Len(t, page.Memories, 1, "the same person in another store of the project holds only that store's facts")
	n, err := f.svc.Clear(admin(f.cells[0]), f.storeB, f.person, "")
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	left, err := f.svc.List(admin(f.cells[0]), f.storeA, f.person, "", "", 50)
	require.NoError(t, err)
	assert.Equal(t, 2, left.Total, "clearing one store leaves the other")
}

func TestService_AStoreOutsideThePersonsProjectIsNotFound(t *testing.T) {
	f := newSvc(t)
	for name, store := range map[string]string{"another project": f.farStore, "unknown": nanoid.MustNew(21)} {
		_, err := f.svc.List(admin(f.cells[0]), store, f.person, "", "", 50)
		assert.True(t, memory.IsStoreNotFoundError(err), name)
		assert.True(t, memory.IsStoreNotFoundError(f.svc.Delete(admin(f.cells[0]), store, f.person, "x")), name)
		_, err = f.svc.Clear(admin(f.cells[0]), store, f.person, "")
		assert.True(t, memory.IsStoreNotFoundError(err), name)
	}
	assert.Zero(t, f.reported)
}

func TestService_AnUnknownPersonIsNotFoundBeforeTheStore(t *testing.T) {
	f := newSvc(t)
	_, err := f.svc.List(admin(f.cells[0]), f.farStore, f.nobody, "", "", 50)
	assert.True(t, memory.IsNotFoundError(err), "the caller learns nothing about a store through an unknown person")
}

func TestService_AStoreWithNoDriverAnswersBackendOff(t *testing.T) {
	f := newSvc(t)
	_, err := f.svc.List(admin(f.cells[0]), f.offStore, f.person, "", "", 50)
	var off *memory.StoreBackendOffError
	require.ErrorAs(t, err, &off)
	assert.Equal(t, memory.Backend("mem0"), off.Backend)
	assert.True(t, memory.IsExpectedError(err), "a 4xx with its own code, never a reported 500")
	assert.Zero(t, f.reported)
}

func TestService_StoreLookupFailureIsReported(t *testing.T) {
	f := newSvc(t)
	f.stores.err = errors.New("db down")
	_, err := f.svc.Clear(admin(f.cells[0]), f.storeA, f.person, "")
	require.Error(t, err)
	assert.Equal(t, 1, f.reported)
}

func TestService_StoreUsable(t *testing.T) {
	f := newSvc(t)
	c := f.cells[0]
	ctx := admin(c)
	require.NoError(t, f.svc.StoreUsable(ctx, c.Scope.ProjectID, c.Scope.StoreID))
	assert.True(t, memory.IsStoreNotFoundError(f.svc.StoreUsable(ctx, uuid.New(), c.Scope.StoreID)), "a store of another project")
	assert.True(t, memory.IsStoreNotFoundError(f.svc.StoreUsable(ctx, c.Scope.ProjectID, uuid.New())), "an unknown store")
	off := f.stores.byPub[f.offStore].ID
	err := f.svc.StoreUsable(ctx, c.Scope.ProjectID, off)
	var inv *memory.InvalidStoreError
	require.ErrorAs(t, err, &inv, "a store whose backend is not on")
	assert.Equal(t, memory.StoreBackendDisabled, inv.Reason)
	assert.Zero(t, f.reported)
	f.stores.err = errors.New("db down")
	require.Error(t, f.svc.StoreUsable(ctx, c.Scope.ProjectID, c.Scope.StoreID))
	assert.Equal(t, 1, f.reported)
}

func TestService_LockStoreShared(t *testing.T) {
	f := newSvc(t)
	c := f.cells[0]
	require.NoError(t, f.svc.LockStoreShared(admin(c), c.Scope.StoreID))
	assert.Equal(t, []uuid.UUID{c.Scope.StoreID}, f.stores.locked)
	assert.True(t, memory.IsStoreNotFoundError(f.svc.LockStoreShared(admin(c), uuid.New())))
	assert.Zero(t, f.reported)
}

func TestService_RunCallsDispatchByTheStoresBackend(t *testing.T) {
	f := newSvc(t)
	c := f.cells[0]
	ctx := c.Ctx
	mode, err := f.svc.Mode(ctx, c.Scope)
	require.NoError(t, err)
	assert.Equal(t, memory.Mode{}, mode, "native does not extract, and a store remembers named API users only when it opts in")
	f.stores.byID[c.Scope.StoreID].RememberAPIUsers = true
	mode, err = f.svc.Mode(ctx, c.Scope)
	require.NoError(t, err)
	assert.Equal(t, memory.Mode{APIUsers: true}, mode)
	require.NoError(t, f.svc.Record(ctx, c.Scope, memory.Turn{Facts: []string{"Prefers tea."}}))
	got, err := f.svc.Recall(ctx, c.Scope, "tea", runtime.MaxMemoryFacts)
	require.NoError(t, err)
	assert.Contains(t, texts(got), "Prefers tea.")
}

func TestService_RunCallsRefuseAStoreTheyCannotUse(t *testing.T) {
	f := newSvc(t)
	c := f.cells[0]
	for name, tc := range map[string]struct {
		mutate func(*memory.Scope)
		check  func(error) bool
	}{
		"another project's store": {func(s *memory.Scope) { s.StoreID = f.cells[8].Scope.StoreID }, memory.IsStoreNotFoundError},
		"an unknown store":        {func(s *memory.Scope) { s.StoreID = uuid.New() }, memory.IsStoreNotFoundError},
		"a disabled backend":      {func(s *memory.Scope) { s.StoreID = f.stores.byPub[f.offStore].ID }, memory.IsInvalidStoreError},
	} {
		s := c.Scope
		tc.mutate(&s)
		_, err := f.svc.Mode(c.Ctx, s)
		assert.True(t, tc.check(err), "%s: extracts %v", name, err)
		_, err = f.svc.Recall(c.Ctx, s, "", runtime.MaxMemoryFacts)
		assert.True(t, tc.check(err), "%s: recall", name)
		assert.True(t, tc.check(f.svc.Record(c.Ctx, s, memory.Turn{Facts: []string{"x"}})), "%s: record", name)
	}
	assert.Zero(t, f.reported, "an unusable store is expected, never reported")
}

func texts(facts []memory.Fact) []string {
	out := make([]string, len(facts))
	for i, f := range facts {
		out[i] = f.Text
	}
	return out
}

func TestService_CreateStore(t *testing.T) {
	f := newSvc(t)
	c := f.cells[0]
	st, err := f.svc.CreateStore(c.Ctx, c.Scope.ProjectID, memory.StoreInput{Name: " Leads ", Backend: memory.BackendNative})
	require.NoError(t, err)
	assert.Equal(t, "Leads", st.Name)
	assert.Equal(t, c.Scope.OrgID, st.OrgID, "the store lands in the org on ctx")
	_, err = f.svc.CreateStore(c.Ctx, c.Scope.ProjectID, memory.StoreInput{Name: "Leads", Backend: memory.BackendNative})
	assert.True(t, memory.IsStoreAlreadyExistsError(err))
	_, err = f.svc.CreateStore(c.Ctx, c.Scope.ProjectID, memory.StoreInput{Name: "Off", Backend: "mem0"})
	assert.True(t, memory.IsInvalidStoreError(err))
	_, err = f.svc.CreateStore(context.Background(), c.Scope.ProjectID, memory.StoreInput{Name: "No tenant", Backend: memory.BackendNative})
	require.Error(t, err)
	f.stores.err = errors.New("db down")
	_, err = f.svc.CreateStore(c.Ctx, c.Scope.ProjectID, memory.StoreInput{Name: "Down", Backend: memory.BackendNative})
	require.Error(t, err)
	assert.Equal(t, 1, f.reported)
	assert.Equal(t, []memory.Backend{memory.BackendNative}, f.svc.Backends())
}

func TestService_ListAndGetNameTheAgentsUsingAStore(t *testing.T) {
	f := newSvc(t)
	c := f.cells[0]
	agentA := memory.UsedBy{AgentID: c.Scope.AgentID, AgentPublicID: f.agentA, Name: "Support"}
	b := &bindings{used: map[uuid.UUID][]memory.UsedBy{c.Scope.StoreID: {agentA}}}
	f.svc.WithBindings(b)
	views, err := f.svc.ListStores(c.Ctx, c.Scope.ProjectID)
	require.NoError(t, err)
	require.Len(t, views, 3, "the project's two stores and the one on a disabled backend")
	for _, v := range views {
		if v.Store.ID == c.Scope.StoreID {
			assert.Equal(t, []memory.UsedBy{agentA}, v.UsedBy)
		} else {
			assert.Empty(t, v.UsedBy)
		}
	}
	got, err := f.svc.GetStore(c.Ctx, c.Scope.StoreID)
	require.NoError(t, err)
	assert.Equal(t, []memory.UsedBy{agentA}, got.UsedBy)
	_, err = f.svc.GetStore(c.Ctx, uuid.New())
	assert.True(t, memory.IsStoreNotFoundError(err))

	ref, err := f.svc.ResolveStore(c.Ctx, f.storeA)
	require.NoError(t, err)
	assert.Equal(t, c.Scope.StoreID, ref.ID)
	_, err = f.svc.ResolveStore(c.Ctx, "unknwnunknwnunknwnabc")
	assert.True(t, memory.IsStoreNotFoundError(err))
	assert.Zero(t, f.reported)

	b.err = errors.New("db down")
	_, err = f.svc.ListStores(c.Ctx, c.Scope.ProjectID)
	require.Error(t, err)
	_, err = f.svc.GetStore(c.Ctx, c.Scope.StoreID)
	require.Error(t, err)
	assert.Equal(t, 2, f.reported)
}

func TestService_DeleteStore(t *testing.T) {
	f := newSvc(t)
	c := f.cells[0]
	b := &bindings{inUse: map[uuid.UUID]bool{c.Scope.StoreID: true}}
	f.svc.WithBindings(b)
	assert.True(t, memory.IsStoreInUseError(f.svc.DeleteStore(c.Ctx, c.Scope.StoreID)), "a store a published version uses stays")
	page, err := f.svc.List(admin(c), f.storeA, f.person, "", "", 50)
	require.NoError(t, err)
	assert.Equal(t, 2, page.Total, "and keeps its facts")

	b.inUse = nil
	require.NoError(t, f.svc.DeleteStore(c.Ctx, c.Scope.StoreID))
	_, err = f.svc.GetStore(c.Ctx, c.Scope.StoreID)
	assert.True(t, memory.IsStoreNotFoundError(err))
	n, err := f.store.ClearStore(c.Ctx, memory.Scope{OrgID: c.Scope.OrgID, ProjectID: c.Scope.ProjectID, StoreID: c.Scope.StoreID})
	require.NoError(t, err)
	assert.Zero(t, n, "the store's facts went with it, through its driver")
	other, err := f.svc.List(admin(c), f.storeB, f.person, "", "", 50)
	require.NoError(t, err)
	assert.Equal(t, 1, other.Total, "another store keeps its facts")

	assert.True(t, memory.IsStoreNotFoundError(f.svc.DeleteStore(c.Ctx, uuid.New())))
	assert.Zero(t, f.reported)
	assert.True(t, memory.IsStoreBackendOffError(f.svc.DeleteStore(c.Ctx, f.stores.byPub[f.offStore].ID)),
		"a store whose driver is not built is never deleted with its facts left behind")
	_, err = f.svc.GetStore(c.Ctx, f.stores.byPub[f.offStore].ID)
	require.NoError(t, err, "and it stays")
	assert.Zero(t, f.reported)
	b.err = errors.New("db down")
	require.Error(t, f.svc.DeleteStore(c.Ctx, f.stores.byPub[f.storeB].ID))
	assert.Equal(t, 1, f.reported)
}
