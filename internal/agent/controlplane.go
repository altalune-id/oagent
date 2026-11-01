package agent

import (
	"context"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	agentv1 "altalune.id/oagent/gen/go/agent/v1"
	agentv1connect "altalune.id/oagent/gen/go/agent/v1/agentv1connect"
	"altalune.id/oagent/internal/kernel/authz"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/kernel/tenant"
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
	return connect.NewResponse(&agentv1.GetAgentResponse{Agent: out}), nil
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
		out = append(out, agentToProto(a, orgPublicID, req.Msg.GetProjectId()))
	}
	return connect.NewResponse(&agentv1.ListAgentsResponse{Agents: out}), nil
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
	a, err = s.agents.UpdateDraft(ctx, a.ID, configFromProto(req.Msg.GetConfig()))
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
	return connect.NewResponse(&agentv1.PublishAgentResponse{Agent: out, Version: versionToProto(v, a.PublicID, by), Created: created}), nil
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
	vs, err := s.agents.Versions(ctx, a.ID)
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
		out = append(out, versionToProto(v, a.PublicID, by))
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
	return agentToProto(a, orgPublicID, projectPublicID), nil
}
