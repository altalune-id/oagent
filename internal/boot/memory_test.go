package boot

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/agent"
	"altalune.id/oagent/internal/enduser"
	"altalune.id/oagent/internal/memory"
	"altalune.id/oagent/internal/memory/memorytest"
	"altalune.id/oagent/internal/memory/native"
	"altalune.id/oagent/internal/run"
	"altalune.id/oagent/internal/testutil/fakes"
	"altalune.id/oagent/nanoid"
)

func TestBuildServices_NativeFactsAlwaysBuilt(t *testing.T) {
	log, _ := bufferLogger()
	for _, on := range []bool{true, false} {
		cfg := devLocalConfig()
		cfg.Memory.Native.Enabled = on
		svcs := buildServices(authKernel(t, log), cfg)
		require.Len(t, svcs.MemoryDrivers, 1)
		assert.IsType(t, &native.Facts{}, svcs.MemoryDrivers[memory.BackendNative], "enabled=%v: admins manage stored facts even with the backend off", on)
		assert.Equal(t, on, len(memoryBackends(cfg)) == 1, "only an enabled backend takes new stores and runs")
	}
}

type oneStore struct {
	memory.Store
	store *memory.MemoryStore
}

func (o oneStore) StoreByID(_ context.Context, id uuid.UUID) (*memory.MemoryStore, error) {
	if id != o.store.ID {
		return nil, &memory.StoreNotFoundError{}
	}
	return o.store, nil
}

func (o oneStore) LockStoreShared(_ context.Context, id uuid.UUID) error {
	if id != o.store.ID {
		return &memory.StoreNotFoundError{}
	}
	return nil
}

func TestRunMemory_MapsScopeAndFacts(t *testing.T) {
	fake := memorytest.NewFake()
	cell := fake.Matrix(t)[0]
	s := cell.Scope
	stores := oneStore{store: &memory.MemoryStore{ID: s.StoreID, OrgID: s.OrgID, ProjectID: s.ProjectID, Backend: memory.BackendNative}}
	svc := memory.NewService(stores, map[memory.Backend]memory.Facts{memory.BackendNative: fake}, []memory.Backend{memory.BackendNative},
		memory.Deps{UOW: fakes.Transactor{}}, apperror.NewReporter(nil, false).Unexpected)
	m := runMemory{memory: svc}
	scope := run.MemoryScope{OrgID: s.OrgID, ProjectID: s.ProjectID, StoreID: s.StoreID, AgentID: s.AgentID, EndUserID: s.EndUserID}
	runID := uuid.New()
	require.NoError(t, m.Record(cell.Ctx, scope, run.MemoryRecord{RunID: runID, Facts: []string{"Lives in Bandung."}}))
	got, err := m.Recall(cell.Ctx, scope, "bandung")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "Lives in Bandung.", got[0].Text)
	mode, err := m.Store(cell.Ctx, scope)
	require.NoError(t, err)
	assert.Equal(t, run.StoreMode{}, mode)
	stores.store.RememberAPIUsers = true
	mode, err = m.Store(cell.Ctx, scope)
	require.NoError(t, err)
	assert.Equal(t, run.StoreMode{APIUsers: true}, mode, "the store's opt-in reaches run")
	facts, _, _, err := fake.List(cell.Ctx, s, "", 10)
	require.NoError(t, err)
	assert.Equal(t, runID, facts[0].SourceRunID)
}

func TestMemoryGrantErr_MapsStoreErrorsToTheAgentsGrantError(t *testing.T) {
	for want, err := range map[agent.GrantReason]error{
		agent.GrantMemoryStore:         &memory.StoreNotFoundError{},
		agent.GrantMemoryStoreDisabled: &memory.InvalidStoreError{Reason: memory.StoreBackendDisabled},
	} {
		var g *agent.InvalidGrantError
		require.ErrorAs(t, memoryGrantErr(err), &g)
		assert.Equal(t, want, g.Reason)
	}
	other := errors.New("db down")
	assert.Equal(t, other, memoryGrantErr(other))
	assert.NoError(t, memoryGrantErr(nil))
}

func TestRememberer_MapsEveryEndUserClass(t *testing.T) {
	assert.Equal(t, run.RememberAlways, rememberer(enduser.MemoryPerson))
	assert.Equal(t, run.RememberNamedAPIUser, rememberer(enduser.MemoryNamedAPIUser))
	assert.Equal(t, run.RememberNever, rememberer(enduser.MemoryNever))
}

func TestMemoryRuns_OneLookupPerPage(t *testing.T) {
	org := uuid.New()
	store := &fakes.RunStore{Runs: map[uuid.UUID]*run.Run{}}
	var ids []uuid.UUID
	for range 5 {
		r := &run.Run{ID: uuid.New(), PublicID: nanoid.MustNew(21), OrgID: org}
		store.Runs[r.ID] = r
		ids = append(ids, r.ID)
	}
	foreign := &run.Run{ID: uuid.New(), PublicID: "foreign", OrgID: uuid.New()}
	store.Runs[foreign.ID] = foreign
	got, err := memoryRuns{svcs: &Services{RunStore: store}}.PublicIDs(context.Background(), org, append(ids, foreign.ID, uuid.New()))
	require.NoError(t, err)
	assert.Equal(t, 1, store.RunPublicIDCalls, "a page of facts from many runs names them in one read")
	assert.Len(t, got, 5, "another org's run and a gone one are left out")
}

func TestMemoryAgents_OneLookupPerPage(t *testing.T) {
	a, b := &agent.Agent{ID: uuid.New(), PublicID: "a1"}, &agent.Agent{ID: uuid.New(), PublicID: "b1"}
	got, err := memoryAgents{store: &fakes.AgentStore{Agents: []*agent.Agent{a, b}}}.PublicIDs(context.Background(), uuid.New(), []uuid.UUID{a.ID, b.ID})
	require.NoError(t, err)
	assert.Equal(t, map[uuid.UUID]string{a.ID: "a1", b.ID: "b1"}, got)
}
