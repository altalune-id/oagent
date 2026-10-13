package agent

import (
	"context"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	"altalune.id/oagent/apperror"
	agentv1 "altalune.id/oagent/gen/go/agent/v1"
	agentv1connect "altalune.id/oagent/gen/go/agent/v1/agentv1connect"
	"altalune.id/oagent/internal/kernel/authz"
)

// AgentService implements agent.v1.AgentService.
type AgentService struct {
	agents *Service
}

// NewControlPlane binds the handler to the agent service.
func NewControlPlane(agents *Service) *AgentService {
	return &AgentService{agents: agents}
}

var _ agentv1connect.AgentServiceHandler = (*AgentService)(nil)

// CreateAgent creates an agent within a project, minting a slug when one is not supplied.
func (s *AgentService) CreateAgent(
	ctx context.Context,
	req *connect.Request[agentv1.CreateAgentRequest],
) (*connect.Response[agentv1.CreateAgentResponse], error) {
	orgID, projectID, err := parseScope(req.Msg.GetOrgId(), req.Msg.GetProjectId())
	if err != nil {
		return nil, err
	}
	if !authz.ReachWholeProject(ctx, orgID, projectID) {
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
	return connect.NewResponse(&agentv1.CreateAgentResponse{Agent: agentToProto(a)}), nil
}

// GetAgent loads an agent by id.
func (s *AgentService) GetAgent(
	ctx context.Context,
	req *connect.Request[agentv1.GetAgentRequest],
) (*connect.Response[agentv1.GetAgentResponse], error) {
	id, err := parseID(req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	a, err := s.reachable(ctx, id)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&agentv1.GetAgentResponse{Agent: agentToProto(a)}), nil
}

// ListAgents lists the project's agents the caller reaches, ordered by creation time.
func (s *AgentService) ListAgents(
	ctx context.Context,
	req *connect.Request[agentv1.ListAgentsRequest],
) (*connect.Response[agentv1.ListAgentsResponse], error) {
	projectID, err := uuid.Parse(req.Msg.GetProjectId())
	if err != nil {
		return nil, apperror.Invalid("project_id", apperror.FieldInvalid).WithCause(err)
	}
	if err := s.reachProject(ctx, projectID); err != nil {
		return nil, err
	}
	agents, err := s.agents.List(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]*agentv1.Agent, 0, len(agents))
	for _, a := range agents {
		if authz.ReachResource(ctx, a.OrgID, a.ProjectID, a.ID) {
			out = append(out, agentToProto(a))
		}
	}
	return connect.NewResponse(&agentv1.ListAgentsResponse{Agents: out}), nil
}

// UpdateAgent updates an agent's name and description.
func (s *AgentService) UpdateAgent(
	ctx context.Context,
	req *connect.Request[agentv1.UpdateAgentRequest],
) (*connect.Response[agentv1.UpdateAgentResponse], error) {
	id, err := parseID(req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	if _, err := s.reachable(ctx, id); err != nil {
		return nil, err
	}
	a, err := s.agents.Update(ctx, id, req.Msg.GetName(), req.Msg.GetDescription())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&agentv1.UpdateAgentResponse{Agent: agentToProto(a)}), nil
}

// DeleteAgent deletes an agent by id.
func (s *AgentService) DeleteAgent(
	ctx context.Context,
	req *connect.Request[agentv1.DeleteAgentRequest],
) (*connect.Response[agentv1.DeleteAgentResponse], error) {
	id, err := parseID(req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	if _, err := s.reachable(ctx, id); err != nil {
		return nil, err
	}
	if err := s.agents.Delete(ctx, id); err != nil {
		return nil, err
	}
	return connect.NewResponse(&agentv1.DeleteAgentResponse{}), nil
}

// SECURITY: an agent outside the caller's reach is reported as absent.
func (s *AgentService) reachable(ctx context.Context, id uuid.UUID) (*Agent, error) {
	a, err := s.agents.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if !authz.ReachResource(ctx, a.OrgID, a.ProjectID, a.ID) {
		return nil, &NotFoundError{ID: id.String()}
	}
	return a, nil
}

// SECURITY: a project outside the caller's reach, or one that does not exist, is reported as an absent agent.
func (s *AgentService) reachProject(ctx context.Context, projectID uuid.UUID) error {
	orgID, ok, err := s.agents.ProjectOrg(ctx, projectID)
	if err != nil || !ok {
		return err
	}
	if !authz.ReachProject(ctx, orgID, projectID) {
		return &NotFoundError{}
	}
	return nil
}

func parseID(raw string) (uuid.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, apperror.Invalid("agent_id", apperror.FieldInvalid).WithCause(err)
	}
	return id, nil
}

func parseScope(orgRaw, projectRaw string) (orgID, projectID uuid.UUID, err error) {
	orgID, err = uuid.Parse(orgRaw)
	if err != nil {
		return uuid.Nil, uuid.Nil, apperror.Invalid("organization_id", apperror.FieldInvalid).WithCause(err)
	}
	projectID, err = uuid.Parse(projectRaw)
	if err != nil {
		return uuid.Nil, uuid.Nil, apperror.Invalid("project_id", apperror.FieldInvalid).WithCause(err)
	}
	return orgID, projectID, nil
}
