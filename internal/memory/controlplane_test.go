package memory_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	memoryv1 "altalune.id/oagent/gen/go/memory/v1"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/memory"
	"altalune.id/oagent/internal/memory/memorytest"
	orgmember "altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/internal/testutil/fakes"
	"altalune.id/oagent/scope"
)

func TestControlPlane_ListDeleteClear(t *testing.T) {
	f := newSvc(t)
	cp := memory.NewControlPlane(f.svc, f.refs)
	ctx := admin(f.cells[0])

	list, err := cp.ListMemories(ctx, connect.NewRequest(&memoryv1.ListMemoriesRequest{StoreId: f.storeA, EndUserId: f.person, PageSize: 1}))
	require.NoError(t, err)
	require.Len(t, list.Msg.GetMemories(), 1)
	assert.Equal(t, int32(2), list.Msg.GetTotal())
	assert.NotEmpty(t, list.Msg.GetNextPageToken())
	m := list.Msg.GetMemories()[0]
	assert.Equal(t, f.runPub, m.GetSourceRunId())
	assert.NotEmpty(t, m.GetAgentId())
	assert.False(t, m.GetCreatedAt().AsTime().IsZero())

	_, err = cp.DeleteMemory(ctx, connect.NewRequest(&memoryv1.DeleteMemoryRequest{StoreId: f.storeA, EndUserId: f.person, Id: m.GetId()}))
	require.NoError(t, err)
	_, err = cp.DeleteMemory(ctx, connect.NewRequest(&memoryv1.DeleteMemoryRequest{StoreId: f.storeA, EndUserId: f.person, Id: m.GetId()}))
	assert.True(t, memory.IsNotFoundError(err))

	cleared, err := cp.ClearMemories(ctx, connect.NewRequest(&memoryv1.ClearMemoriesRequest{StoreId: f.storeA, EndUserId: f.person}))
	require.NoError(t, err)
	assert.Equal(t, int32(1), cleared.Msg.GetDeleted())

	_, err = cp.ListMemories(ctx, connect.NewRequest(&memoryv1.ListMemoriesRequest{StoreId: f.storeA, EndUserId: f.nobody}))
	assert.True(t, memory.IsNotFoundError(err))
	_, err = cp.ClearMemories(ctx, connect.NewRequest(&memoryv1.ClearMemoriesRequest{StoreId: f.storeA, EndUserId: f.nobody}))
	assert.True(t, memory.IsNotFoundError(err))
}

func TestNotFound_IsMEM001(t *testing.T) {
	assert.Equal(t, "MEM001", string((&memory.NotFoundError{ID: "x"}).ToAppError().Code()))
}

func TestStoreErrors_AreMEM002To005(t *testing.T) {
	for want, e := range map[string]interface{ ToAppError() *apperror.AppError }{
		"MEM002": &memory.StoreNotFoundError{},
		"MEM003": &memory.InvalidStoreError{Reason: memory.StoreBackendDisabled},
		"MEM004": &memory.StoreInUseError{},
		"MEM005": &memory.StoreAlreadyExistsError{Name: "People"},
	} {
		assert.Equal(t, want, string(e.ToAppError().Code()))
	}
}

func member(c memorytest.Cell) context.Context {
	return session.PrincipalInto(context.Background(), session.Principal{
		ActiveOrgID: c.Scope.OrgID, UserID: uuid.New(), Source: session.SourceLocal, Scopes: orgmember.BuiltinPermissions(orgmember.RoleMember),
	})
}

func TestControlPlane_StoreCreateListGetDelete(t *testing.T) {
	f := newSvc(t)
	c := f.cells[0]
	f.svc.WithBindings(&bindings{used: map[uuid.UUID][]memory.UsedBy{c.Scope.StoreID: {{AgentID: c.Scope.AgentID, AgentPublicID: f.agentA, Name: "Support"}}}})
	cp := memory.NewControlPlane(f.svc, f.refs)
	ctx := fakes.OperatorContext(context.Background())

	created, err := cp.CreateMemoryStore(ctx, connect.NewRequest(&memoryv1.CreateMemoryStoreRequest{OrgId: f.orgPub, ProjectId: f.prjPub, Name: "Leads"}))
	require.NoError(t, err)
	st := created.Msg.GetStore()
	assert.Equal(t, memoryv1.MemoryBackend_MEMORY_BACKEND_NATIVE, st.GetBackend(), "an unset backend takes the only enabled one")
	assert.Equal(t, f.orgPub, st.GetOrgId())
	assert.Equal(t, f.prjPub, st.GetProjectId())
	assert.Len(t, st.GetId(), 21)
	assert.False(t, st.GetRememberApiUsers(), "named API users are off unless asked for")
	opted, err := cp.CreateMemoryStore(ctx, connect.NewRequest(&memoryv1.CreateMemoryStoreRequest{
		OrgId: f.orgPub, ProjectId: f.prjPub, Name: "API users", RememberApiUsers: true,
	}))
	require.NoError(t, err)
	assert.True(t, opted.Msg.GetStore().GetRememberApiUsers())

	list, err := cp.ListMemoryStores(ctx, connect.NewRequest(&memoryv1.ListMemoryStoresRequest{ProjectId: f.prjPub}))
	require.NoError(t, err)
	assert.Equal(t, []memoryv1.MemoryBackend{memoryv1.MemoryBackend_MEMORY_BACKEND_NATIVE}, list.Msg.GetEnabledBackends())
	require.Len(t, list.Msg.GetStores(), 5)
	var usedBy []*memoryv1.MemoryStoreUsedBy
	for _, s := range list.Msg.GetStores() {
		if s.GetId() == f.storeA {
			usedBy = s.GetUsedBy()
		}
	}
	require.Len(t, usedBy, 1)
	assert.Equal(t, f.agentA, usedBy[0].GetAgentId(), "agents by public id")

	got, err := cp.GetMemoryStore(ctx, connect.NewRequest(&memoryv1.GetMemoryStoreRequest{Id: f.storeA}))
	require.NoError(t, err)
	assert.Equal(t, "Support", got.Msg.GetStore().GetUsedBy()[0].GetAgentName())
	assert.Equal(t, f.prjPub, got.Msg.GetStore().GetProjectId())

	_, err = cp.DeleteMemoryStore(ctx, connect.NewRequest(&memoryv1.DeleteMemoryStoreRequest{Id: st.GetId()}))
	require.NoError(t, err)
	_, err = cp.GetMemoryStore(ctx, connect.NewRequest(&memoryv1.GetMemoryStoreRequest{Id: st.GetId()}))
	assert.True(t, memory.IsStoreNotFoundError(err))
}

func TestControlPlane_StoresOutOfReachAreNotFound(t *testing.T) {
	f := newSvc(t)
	cp := memory.NewControlPlane(f.svc, f.refs)
	stranger := session.PrincipalInto(context.Background(), session.Principal{ActiveOrgID: uuid.New(), UserID: uuid.New()})
	_, err := cp.GetMemoryStore(stranger, connect.NewRequest(&memoryv1.GetMemoryStoreRequest{Id: f.storeA}))
	assert.True(t, memory.IsStoreNotFoundError(err), "another org's store answers like an unknown one")
	_, err = cp.DeleteMemoryStore(stranger, connect.NewRequest(&memoryv1.DeleteMemoryStoreRequest{Id: f.storeA}))
	assert.True(t, memory.IsStoreNotFoundError(err))
	_, err = cp.ListMemoryStores(stranger, connect.NewRequest(&memoryv1.ListMemoryStoresRequest{ProjectId: f.prjPub}))
	assert.True(t, memory.IsStoreNotFoundError(err))
	_, err = cp.CreateMemoryStore(stranger, connect.NewRequest(&memoryv1.CreateMemoryStoreRequest{OrgId: f.orgPub, ProjectId: f.prjPub, Name: "X"}))
	assert.True(t, memory.IsStoreNotFoundError(err))
	_, err = cp.GetMemoryStore(admin(f.cells[0]), connect.NewRequest(&memoryv1.GetMemoryStoreRequest{Id: "unknwnunknwnunknwnabc"}))
	assert.True(t, memory.IsStoreNotFoundError(err))
	_, err = cp.ListMemoryStores(admin(f.cells[0]), connect.NewRequest(&memoryv1.ListMemoryStoresRequest{ProjectId: "unknwnunknwnunknwnabc"}))
	assert.True(t, memory.IsStoreNotFoundError(err))
	_, err = cp.CreateMemoryStore(admin(f.cells[0]), connect.NewRequest(&memoryv1.CreateMemoryStoreRequest{OrgId: f.orgPub, ProjectId: f.farPrjPub, Name: "X"}))
	assert.True(t, memory.IsStoreNotFoundError(err), "a project of another org than the one named")
}

func TestControlPlane_AgentNamesNeedAgentRead(t *testing.T) {
	f := newSvc(t)
	c := f.cells[0]
	f.svc.WithBindings(&bindings{used: map[uuid.UUID][]memory.UsedBy{c.Scope.StoreID: {{AgentID: c.Scope.AgentID, AgentPublicID: f.agentA, Name: "Support"}}}})
	cp := memory.NewControlPlane(f.svc, f.refs)
	noAgents := session.PrincipalInto(context.Background(), session.Principal{
		ActiveOrgID: c.Scope.OrgID, UserID: uuid.New(), Source: session.SourceLocal, Scopes: []string{scope.MemoryRead},
	})
	got, err := cp.GetMemoryStore(noAgents, connect.NewRequest(&memoryv1.GetMemoryStoreRequest{Id: f.storeA}))
	require.NoError(t, err)
	assert.Empty(t, got.Msg.GetStore().GetUsedBy(), "a caller who cannot read agents never sees their names")
	got, err = cp.GetMemoryStore(member(c), connect.NewRequest(&memoryv1.GetMemoryStoreRequest{Id: f.storeA}))
	require.NoError(t, err)
	assert.Len(t, got.Msg.GetStore().GetUsedBy(), 1, "a member reads agents")
}

func TestControlPlane_AKeyLimitedToOneStoreSeesOnlyIt(t *testing.T) {
	f := newSvc(t)
	c := f.cells[0]
	cp := memory.NewControlPlane(f.svc, f.refs)
	limited := session.PrincipalInto(context.Background(), session.Principal{
		Source: session.SourceAPIKey, ActiveOrgID: c.Scope.OrgID, ProjectIDs: []uuid.UUID{c.Scope.ProjectID},
		ResourceIDs: []uuid.UUID{c.Scope.StoreID}, Scopes: []string{scope.MemoryRead, scope.MemoryWrite},
	})
	list, err := cp.ListMemoryStores(limited, connect.NewRequest(&memoryv1.ListMemoryStoresRequest{ProjectId: f.prjPub}))
	require.NoError(t, err)
	require.Len(t, list.Msg.GetStores(), 1, "the other stores of the project stay hidden")
	assert.Equal(t, f.storeA, list.Msg.GetStores()[0].GetId())
	_, err = cp.GetMemoryStore(limited, connect.NewRequest(&memoryv1.GetMemoryStoreRequest{Id: f.storeB}))
	assert.True(t, memory.IsStoreNotFoundError(err))
	_, err = cp.CreateMemoryStore(limited, connect.NewRequest(&memoryv1.CreateMemoryStoreRequest{OrgId: f.orgPub, ProjectId: f.prjPub, Name: "X"}))
	assert.True(t, memory.IsStoreNotFoundError(err), "a key limited to resources never creates across the project")
}
