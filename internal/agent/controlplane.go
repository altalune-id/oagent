package agent

import (
	"context"
	"slices"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	agentv1 "altalune.id/oagent/gen/go/agent/v1"
	agentv1connect "altalune.id/oagent/gen/go/agent/v1/agentv1connect"
	"altalune.id/oagent/internal/kernel/authz"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/scope"
)

// AgentService implements agent.v1.AgentService; every id it takes and returns is a public id.
type AgentService struct {
	agents *Service
	refs   Refs
}

// NewControlPlane binds the handler to the agent service and the org and project public id lookups.
func NewControlPlane(agents *Service, refs Refs) *AgentService {
	return &AgentService{agents: agents, refs: refs}
}

var _ agentv1connect.AgentServiceHandler = (*AgentService)(nil)

// CreateAgent creates an agent within a project, minting a slug when one is not supplied.
func (s *AgentService) CreateAgent(
	ctx context.Context,
	req *connect.Request[agentv1.CreateAgentRequest],
) (*connect.Response[agentv1.CreateAgentResponse], error) {
	orgID, ok, err := s.refs.OrgID(ctx, req.Msg.GetOrgId())
	if err != nil {
		return nil, err
	}
	ctx, projectID, err := s.project(ctx, req.Msg.GetProjectId(), authz.ReachWholeProject)
	if err != nil {
		return nil, err
	}
	if !ok || !authz.ReachWholeProject(ctx, orgID, projectID) {
		return nil, &NotFoundError{}
	}
	a, err := s.agents.Create(ctx, CreateRequest{
		OrgID:       orgID,
		ProjectID:   projectID,
		Slug:        req.Msg.GetSlug(),
		Name:        req.Msg.GetName(),
		Description: req.Msg.GetDescription(),
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&agentv1.CreateAgentResponse{
		Agent: agentToProto(a, req.Msg.GetOrgId(), req.Msg.GetProjectId()),
	}), nil
}

// GetAgent loads an agent by id.
func (s *AgentService) GetAgent(
	ctx context.Context,
	req *connect.Request[agentv1.GetAgentRequest],
) (*connect.Response[agentv1.GetAgentResponse], error) {
	ctx, a, err := s.reachable(ctx, req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	out, err := s.toProto(ctx, a)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&agentv1.GetAgentResponse{Agent: out, Defaults: defaultsToProto()}), nil
}

// ListAgents lists the project's agents the caller reaches, ordered by creation time.
func (s *AgentService) ListAgents(
	ctx context.Context,
	req *connect.Request[agentv1.ListAgentsRequest],
) (*connect.Response[agentv1.ListAgentsResponse], error) {
	ctx, projectID, err := s.project(ctx, req.Msg.GetProjectId(), authz.ReachProject)
	if err != nil {
		return nil, err
	}
	agents, err := s.agents.List(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]*agentv1.Agent, 0, len(agents))
	orgPublicID := ""
	for _, a := range agents {
		if !authz.ReachResource(ctx, a.OrgID, a.ProjectID, a.ID) {
			continue
		}
		if orgPublicID == "" {
			if orgPublicID, err = s.refs.OrgPublicID(ctx, a.OrgID); err != nil {
				return nil, err
			}
		}
		item := agentToProto(a, orgPublicID, req.Msg.GetProjectId())
		if err := s.fillRefs(ctx, a.Draft, item.Draft); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return connect.NewResponse(&agentv1.ListAgentsResponse{Agents: out, Defaults: defaultsToProto()}), nil
}

// UpdateAgent updates an agent's name and description.
func (s *AgentService) UpdateAgent(
	ctx context.Context,
	req *connect.Request[agentv1.UpdateAgentRequest],
) (*connect.Response[agentv1.UpdateAgentResponse], error) {
	ctx, a, err := s.reachable(ctx, req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	a, err = s.agents.Update(ctx, a.ID, req.Msg.GetName(), req.Msg.GetDescription())
	if err != nil {
		return nil, err
	}
	out, err := s.toProto(ctx, a)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&agentv1.UpdateAgentResponse{Agent: out}), nil
}

// DeleteAgent deletes an agent by id.
func (s *AgentService) DeleteAgent(
	ctx context.Context,
	req *connect.Request[agentv1.DeleteAgentRequest],
) (*connect.Response[agentv1.DeleteAgentResponse], error) {
	ctx, a, err := s.reachable(ctx, req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	if err := s.agents.Delete(ctx, a.ID); err != nil {
		return nil, err
	}
	return connect.NewResponse(&agentv1.DeleteAgentResponse{}), nil
}

// UpdateAgentDraft replaces an agent's draft configuration.
func (s *AgentService) UpdateAgentDraft(
	ctx context.Context,
	req *connect.Request[agentv1.UpdateAgentDraftRequest],
) (*connect.Response[agentv1.UpdateAgentDraftResponse], error) {
	ctx, a, err := s.reachable(ctx, req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	cfg, err := s.configFrom(ctx, a, req.Msg.GetConfig())
	if err != nil {
		return nil, err
	}
	a, err = s.agents.UpdateDraft(ctx, a.ID, cfg)
	if err != nil {
		return nil, err
	}
	out, err := s.toProto(ctx, a)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&agentv1.UpdateAgentDraftResponse{Agent: out}), nil
}

// PublishAgent publishes an agent's draft as its next version, as the calling person.
func (s *AgentService) PublishAgent(
	ctx context.Context,
	req *connect.Request[agentv1.PublishAgentRequest],
) (*connect.Response[agentv1.PublishAgentResponse], error) {
	ctx, a, err := s.reachable(ctx, req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	a, v, created, err := s.agents.Publish(ctx, a.ID, session.PrincipalFrom(ctx).UserID)
	if err != nil {
		return nil, err
	}
	out, err := s.toProto(ctx, a)
	if err != nil {
		return nil, err
	}
	by, err := s.userPublicID(ctx, v.CreatedBy)
	if err != nil {
		return nil, err
	}
	version, err := s.versionOut(ctx, v, a.PublicID, by)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&agentv1.PublishAgentResponse{Agent: out, Version: version, Created: created}), nil
}

// ListAgentVersions lists an agent's published versions, newest first.
func (s *AgentService) ListAgentVersions(
	ctx context.Context,
	req *connect.Request[agentv1.ListAgentVersionsRequest],
) (*connect.Response[agentv1.ListAgentVersionsResponse], error) {
	ctx, a, err := s.reachable(ctx, req.Msg.GetAgentId())
	if err != nil {
		return nil, err
	}
	vs, err := s.agents.Versions(ctx, a.ID, int(req.Msg.GetLimit()))
	if err != nil {
		return nil, err
	}
	people := map[uuid.UUID]string{}
	out := make([]*agentv1.AgentVersion, 0, len(vs))
	for _, v := range vs {
		by, ok := people[v.CreatedBy.UUID]
		if !ok {
			if by, err = s.userPublicID(ctx, v.CreatedBy); err != nil {
				return nil, err
			}
			people[v.CreatedBy.UUID] = by
		}
		version, err := s.versionOut(ctx, v, a.PublicID, by)
		if err != nil {
			return nil, err
		}
		out = append(out, version)
	}
	return connect.NewResponse(&agentv1.ListAgentVersionsResponse{Versions: out}), nil
}

func (s *AgentService) userPublicID(ctx context.Context, id uuid.NullUUID) (string, error) {
	if !id.Valid {
		return "", nil
	}
	return s.refs.UserPublicID(ctx, id.UUID)
}

// SECURITY: an unknown agent and an agent outside the caller's reach answer the same not-found; a reached agent scopes the call to its org.
func (s *AgentService) reachable(ctx context.Context, publicID string) (context.Context, *Agent, error) {
	r, err := s.agents.Resolve(ctx, publicID)
	if IsNotFoundError(err) {
		return ctx, nil, &NotFoundError{}
	}
	if err != nil {
		return ctx, nil, err
	}
	if !authz.ReachResource(ctx, r.OrgID, r.ProjectID, r.ID) {
		return ctx, nil, &NotFoundError{}
	}
	ctx = tenant.WithOrg(ctx, r.OrgID)
	a, err := s.agents.Get(ctx, r.ID)
	if IsNotFoundError(err) {
		return ctx, nil, &NotFoundError{}
	}
	return ctx, a, err
}

// SECURITY: an unknown or unreachable project is reported as an absent agent; a reached project scopes the call to its org.
func (s *AgentService) project(
	ctx context.Context, publicID string, reach func(ctx context.Context, orgID, projectID uuid.UUID) bool,
) (context.Context, uuid.UUID, error) {
	id, orgID, ok, err := s.refs.ProjectID(ctx, publicID)
	if err != nil {
		return ctx, uuid.Nil, err
	}
	if !ok || !reach(ctx, orgID, id) {
		return ctx, uuid.Nil, &NotFoundError{}
	}
	return tenant.WithOrg(ctx, orgID), id, nil
}

func (s *AgentService) toProto(ctx context.Context, a *Agent) (*agentv1.Agent, error) {
	orgPublicID, err := s.refs.OrgPublicID(ctx, a.OrgID)
	if err != nil {
		return nil, err
	}
	projectPublicID, err := s.refs.ProjectPublicID(ctx, a.ProjectID)
	if err != nil {
		return nil, err
	}
	out := agentToProto(a, orgPublicID, projectPublicID)
	if err := s.fillRefs(ctx, a.Draft, out.Draft); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *AgentService) versionOut(ctx context.Context, v *Version, agentPublicID, by string) (*agentv1.AgentVersion, error) {
	out := versionToProto(v, agentPublicID, by)
	if err := s.fillRefs(ctx, v.Config, out.Config); err != nil {
		return nil, err
	}
	return out, nil
}

// SECURITY: a reference that no longer resolves renders empty and flagged, never as its UUID and never as an error.
func (s *AgentService) fillRefs(ctx context.Context, c Config, out *agentv1.AgentConfig) error {
	if c.ProviderID != uuid.Nil {
		pub, ok, err := s.refs.ProviderPublicID(ctx, c.ProviderID)
		if err != nil {
			return err
		}
		out.ProviderId, out.ProviderMissing = pub, !ok
	}
	for i, g := range c.MCPServers {
		pub, ok, err := s.refs.MCPServerPublicID(ctx, g.ServerID)
		if err != nil {
			return err
		}
		out.McpServers[i].ServerId, out.McpServers[i].Missing = pub, !ok
	}
	for _, id := range c.KnowledgeSourceIDs {
		pub, ok, err := s.refs.KnowledgeSourcePublicID(ctx, id)
		if err != nil {
			return err
		}
		if !ok {
			out.KnowledgeSourcesMissing++
			continue
		}
		out.KnowledgeSourceIds = append(out.KnowledgeSourceIds, pub)
	}
	if c.MemoryStoreID != uuid.Nil {
		pub, ok, err := s.refs.MemoryStorePublicID(ctx, c.MemoryStoreID)
		if err != nil {
			return err
		}
		out.MemoryStoreId, out.MemoryStoreMissing = pub, !ok
	}
	return nil
}

// SECURITY: a provider or server of another project answers the same as an unknown one.
func (s *AgentService) configFrom(ctx context.Context, a *Agent, in *agentv1.AgentConfig) (Config, error) {
	cfg := configFromProto(in)
	if pub := in.GetProviderId(); pub != "" {
		id, owner, ok, err := s.refs.ProviderID(ctx, pub)
		if err != nil {
			return Config{}, err
		}
		if !ok || owner != a.ProjectID || !mayBind(ctx, a, id, a.Draft.ProviderID == id, scope.ProviderRead) {
			return Config{}, &InvalidGrantError{Reason: GrantProvider}
		}
		cfg.ProviderID = id
	}
	for i, g := range in.GetMcpServers() {
		id, owner, ok, err := s.refs.MCPServerID(ctx, g.GetServerId())
		if err != nil {
			return Config{}, err
		}
		bound := slices.IndexFunc(a.Draft.MCPServers, func(m MCPGrant) bool { return m.ServerID == id })
		onDraft := bound >= 0 && !widens(a.Draft.MCPServers[bound], cfg.MCPServers[i])
		if !ok || owner != a.ProjectID || !mayBind(ctx, a, id, onDraft, scope.McpserverRead) {
			return Config{}, &InvalidGrantError{Reason: GrantServer}
		}
		cfg.MCPServers[i].ServerID = id
	}
	for _, pub := range in.GetKnowledgeSourceIds() {
		id, owner, ok, err := s.refs.KnowledgeSourceID(ctx, pub)
		if err != nil {
			return Config{}, err
		}
		if !ok || owner != a.ProjectID || !mayBind(ctx, a, id, slices.Contains(a.Draft.KnowledgeSourceIDs, id), scope.KnowledgeRead) {
			return Config{}, &InvalidGrantError{Reason: GrantKnowledgeSource}
		}
		cfg.KnowledgeSourceIDs = append(cfg.KnowledgeSourceIDs, id)
	}
	if pub := in.GetMemoryStoreId(); pub != "" {
		id, owner, ok, err := s.refs.MemoryStoreID(ctx, pub)
		if err != nil {
			return Config{}, err
		}
		if !ok || owner != a.ProjectID || !mayBind(ctx, a, id, a.Draft.MemoryStoreID == id, scope.MemoryRead) {
			return Config{}, &InvalidGrantError{Reason: GrantMemoryStore}
		}
		cfg.MemoryStoreID = id
	}
	return cfg, nil
}

// SECURITY: a new tool, an approval turned off, or a gated tool's free_when added or changed widens a bound grant; the rest narrows it.
func widens(current, next MCPGrant) bool {
	for _, t := range next.Tools {
		i := slices.IndexFunc(current.Tools, func(c Tool) bool { return c.Name == t.Name })
		if i < 0 {
			return true
		}
		was := current.Tools[i]
		switch {
		case was.RequireApproval && !t.RequireApproval:
			return true
		case was.RequireApproval && len(t.FreeWhen) > 0 && !slices.EqualFunc(was.FreeWhen, t.FreeWhen, FreeCondition.Equal):
			return true
		}
	}
	return false
}

// SECURITY: a reference new to the draft needs reach on it and its read scope, so an agent-scoped key cannot borrow project credentials.
func mayBind(ctx context.Context, a *Agent, refID uuid.UUID, onDraft bool, readScope string) bool {
	if onDraft {
		return true
	}
	return authz.ReachResource(ctx, a.OrgID, a.ProjectID, refID) && authz.Allowed(session.PrincipalFrom(ctx), readScope)
}
