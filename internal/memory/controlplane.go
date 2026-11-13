package memory

import (
	"context"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	memoryv1 "altalune.id/oagent/gen/go/memory/v1"
	"altalune.id/oagent/gen/go/memory/v1/memoryv1connect"
	"altalune.id/oagent/internal/kernel/authz"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/scope"
)

const defaultPageSize = 50

// Refs turns org and project public ids into ids and back for the handler; ok is false for an unknown id.
type Refs interface {
	OrgID(ctx context.Context, publicID string) (id uuid.UUID, ok bool, err error)
	OrgPublicID(ctx context.Context, id uuid.UUID) (string, error)
	ProjectID(ctx context.Context, publicID string) (id, orgID uuid.UUID, ok bool, err error)
	ProjectPublicID(ctx context.Context, id uuid.UUID) (string, error)
}

// MemoryService implements memory.v1.MemoryService; every id it takes and returns is a public id.
type MemoryService struct {
	memories *Service
	refs     Refs
}

// NewControlPlane binds the handler to the memory service and the public id lookups.
func NewControlPlane(memories *Service, refs Refs) *MemoryService {
	return &MemoryService{memories: memories, refs: refs}
}

// CreateMemoryStore adds a store to a project.
func (s *MemoryService) CreateMemoryStore(
	ctx context.Context, req *connect.Request[memoryv1.CreateMemoryStoreRequest],
) (*connect.Response[memoryv1.CreateMemoryStoreResponse], error) {
	m := req.Msg
	orgID, ok, err := s.refs.OrgID(ctx, m.GetOrgId())
	if err != nil {
		return nil, err
	}
	ctx, projectID, err := s.project(ctx, m.GetProjectId(), authz.ReachWholeProject)
	if err != nil {
		return nil, err
	}
	// SECURITY: a project of another org answers the same not-found as an unknown one.
	if tc, terr := tenant.From(ctx); !ok || terr != nil || tc.OrgID != orgID {
		return nil, &StoreNotFoundError{}
	}
	store, err := s.memories.CreateStore(ctx, projectID, StoreInput{
		Name: m.GetName(), Backend: s.backendOf(m.GetBackend()), RememberAPIUsers: m.GetRememberApiUsers(),
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&memoryv1.CreateMemoryStoreResponse{
		Store: storeToProto(StoreView{Store: store}, m.GetOrgId(), m.GetProjectId(), nil),
	}), nil
}

func (s *MemoryService) backendOf(b memoryv1.MemoryBackend) Backend {
	if b == memoryv1.MemoryBackend_MEMORY_BACKEND_NATIVE {
		return BackendNative
	}
	if enabled := s.memories.Backends(); len(enabled) == 1 {
		return enabled[0]
	}
	return ""
}

// ListMemoryStores lists a project's stores with the agents that use each and the backends this server runs.
func (s *MemoryService) ListMemoryStores(
	ctx context.Context, req *connect.Request[memoryv1.ListMemoryStoresRequest],
) (*connect.Response[memoryv1.ListMemoryStoresResponse], error) {
	ctx, projectID, err := s.project(ctx, req.Msg.GetProjectId(), authz.ReachProject)
	if err != nil {
		return nil, err
	}
	views, err := s.memories.ListStores(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := &memoryv1.ListMemoryStoresResponse{}
	for _, b := range s.memories.Backends() {
		out.EnabledBackends = append(out.EnabledBackends, backendToProto(b))
	}
	orgPub, shown := "", shownAgents(ctx)
	for _, v := range views {
		if !authz.ReachResource(ctx, v.Store.OrgID, v.Store.ProjectID, v.Store.ID) {
			continue
		}
		if orgPub == "" {
			if orgPub, err = s.refs.OrgPublicID(ctx, v.Store.OrgID); err != nil {
				return nil, err
			}
		}
		out.Stores = append(out.Stores, storeToProto(v, orgPub, req.Msg.GetProjectId(), shown))
	}
	return connect.NewResponse(out), nil
}

// GetMemoryStore loads a store by id with the agents that use it.
func (s *MemoryService) GetMemoryStore(
	ctx context.Context, req *connect.Request[memoryv1.GetMemoryStoreRequest],
) (*connect.Response[memoryv1.GetMemoryStoreResponse], error) {
	ctx, ref, err := s.reachableStore(ctx, req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	v, err := s.memories.GetStore(ctx, ref.ID)
	if err != nil {
		return nil, err
	}
	orgPub, err := s.refs.OrgPublicID(ctx, ref.OrgID)
	if err != nil {
		return nil, err
	}
	projectPub, err := s.refs.ProjectPublicID(ctx, ref.ProjectID)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&memoryv1.GetMemoryStoreResponse{Store: storeToProto(v, orgPub, projectPub, shownAgents(ctx))}), nil
}

// DeleteMemoryStore deletes a store no published agent version uses, with every fact in it.
func (s *MemoryService) DeleteMemoryStore(
	ctx context.Context, req *connect.Request[memoryv1.DeleteMemoryStoreRequest],
) (*connect.Response[memoryv1.DeleteMemoryStoreResponse], error) {
	ctx, ref, err := s.reachableStore(ctx, req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	if err := s.memories.DeleteStore(ctx, ref.ID); err != nil {
		return nil, err
	}
	return connect.NewResponse(&memoryv1.DeleteMemoryStoreResponse{}), nil
}

// SECURITY: unknown and out-of-reach stores answer the same not-found; a reached store scopes the call to its org and project.
func (s *MemoryService) reachableStore(ctx context.Context, publicID string) (context.Context, Ref, error) {
	ref, err := s.memories.ResolveStore(ctx, publicID)
	if err != nil {
		return ctx, Ref{}, err
	}
	if !authz.ReachResource(ctx, ref.OrgID, ref.ProjectID, ref.ID) {
		return ctx, Ref{}, &StoreNotFoundError{}
	}
	return scoped(ctx, ref.OrgID, ref.ProjectID), ref, nil
}

// SECURITY: an unknown or unreachable project is reported as an absent store; a reached project scopes the call to its org and itself.
func (s *MemoryService) project(
	ctx context.Context, publicID string, reach func(ctx context.Context, orgID, projectID uuid.UUID) bool,
) (context.Context, uuid.UUID, error) {
	id, orgID, ok, err := s.refs.ProjectID(ctx, publicID)
	if err != nil {
		return ctx, uuid.Nil, err
	}
	if !ok || !reach(ctx, orgID, id) {
		return ctx, uuid.Nil, &StoreNotFoundError{}
	}
	return scoped(ctx, orgID, id), id, nil
}

func scoped(ctx context.Context, orgID, projectID uuid.UUID) context.Context {
	return tenant.WithProject(tenant.WithOrg(ctx, orgID), projectID)
}

// SECURITY: an agent is named only to a caller who may read agents and reaches that one.
func shownAgents(ctx context.Context) func(*MemoryStore, UsedBy) bool {
	if !authz.Allowed(session.PrincipalFrom(ctx), scope.AgentRead) {
		return nil
	}
	return func(st *MemoryStore, u UsedBy) bool {
		return authz.ReachResource(ctx, st.OrgID, st.ProjectID, u.AgentID)
	}
}

func storeToProto(v StoreView, orgPub, projectPub string, shown func(*MemoryStore, UsedBy) bool) *memoryv1.MemoryStore {
	st := v.Store
	out := &memoryv1.MemoryStore{
		Id: st.PublicID, OrgId: orgPub, ProjectId: projectPub, Name: st.Name, Backend: backendToProto(st.Backend),
		CreatedAt: timestamppb.New(st.CreatedAt), UpdatedAt: timestamppb.New(st.UpdatedAt), RememberApiUsers: st.RememberAPIUsers,
	}
	if shown == nil {
		return out
	}
	for _, u := range v.UsedBy {
		if shown(st, u) {
			out.UsedBy = append(out.UsedBy, &memoryv1.MemoryStoreUsedBy{AgentId: u.AgentPublicID, AgentName: u.Name})
		}
	}
	return out
}

func count(n int) int32 { return int32(min(n, 1<<30)) } //nolint:gosec // G115: clamped.

func backendToProto(b Backend) memoryv1.MemoryBackend {
	if b == BackendNative {
		return memoryv1.MemoryBackend_MEMORY_BACKEND_NATIVE
	}
	return memoryv1.MemoryBackend_MEMORY_BACKEND_UNSPECIFIED
}

var _ memoryv1connect.MemoryServiceHandler = (*MemoryService)(nil)

// ListMemories lists an end user's facts in a store newest first.
func (s *MemoryService) ListMemories(
	ctx context.Context, req *connect.Request[memoryv1.ListMemoriesRequest],
) (*connect.Response[memoryv1.ListMemoriesResponse], error) {
	m := req.Msg
	size := int(m.GetPageSize())
	if size == 0 {
		size = defaultPageSize
	}
	page, err := s.memories.List(ctx, m.GetStoreId(), m.GetEndUserId(), m.GetAgentId(), m.GetPageToken(), size)
	if err != nil {
		return nil, err
	}
	total := count(page.Total)
	out := &memoryv1.ListMemoriesResponse{NextPageToken: page.Next, Total: total}
	for _, f := range page.Memories {
		out.Memories = append(out.Memories, &memoryv1.Memory{
			Id: f.ID, Text: f.Text, AgentId: f.AgentID, CreatedAt: timestamppb.New(f.CreatedAt), SourceRunId: f.SourceRunID,
		})
	}
	return connect.NewResponse(out), nil
}

// DeleteMemory deletes one fact of an end user in a store.
func (s *MemoryService) DeleteMemory(
	ctx context.Context, req *connect.Request[memoryv1.DeleteMemoryRequest],
) (*connect.Response[memoryv1.DeleteMemoryResponse], error) {
	if err := s.memories.Delete(ctx, req.Msg.GetStoreId(), req.Msg.GetEndUserId(), req.Msg.GetId()); err != nil {
		return nil, err
	}
	return connect.NewResponse(&memoryv1.DeleteMemoryResponse{}), nil
}

// ClearMemories deletes an end user's facts in a store, every agent's or one agent's.
func (s *MemoryService) ClearMemories(
	ctx context.Context, req *connect.Request[memoryv1.ClearMemoriesRequest],
) (*connect.Response[memoryv1.ClearMemoriesResponse], error) {
	n, err := s.memories.Clear(ctx, req.Msg.GetStoreId(), req.Msg.GetEndUserId(), req.Msg.GetAgentId())
	if err != nil {
		return nil, err
	}
	deleted := count(n)
	return connect.NewResponse(&memoryv1.ClearMemoriesResponse{Deleted: deleted}), nil
}
