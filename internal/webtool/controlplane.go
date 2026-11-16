package webtool

import (
	"context"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	webtoolv1 "altalune.id/oagent/gen/go/webtool/v1"
	"altalune.id/oagent/gen/go/webtool/v1/webtoolv1connect"
	"altalune.id/oagent/internal/kernel/authz"
	"altalune.id/oagent/internal/kernel/tenant"
)

// WebToolService implements webtool.v1.WebToolService; every id it takes and returns is a public id, and no key or value leaves it.
type WebToolService struct {
	svc  *Service
	refs Refs
}

// NewControlPlane binds the handler to the web tool service and the project public id lookups.
func NewControlPlane(svc *Service, refs Refs) *WebToolService {
	return &WebToolService{svc: svc, refs: refs}
}

var _ webtoolv1connect.WebToolServiceHandler = (*WebToolService)(nil)

// GetWebSearchProvider loads the project's search provider and the kinds this deployment offers.
func (s *WebToolService) GetWebSearchProvider(
	ctx context.Context, req *connect.Request[webtoolv1.GetWebSearchProviderRequest],
) (*connect.Response[webtoolv1.GetWebSearchProviderResponse], error) {
	ctx, projectID, err := s.project(ctx, req.Msg.GetProjectId(), authz.ReachProject)
	if err != nil {
		return nil, err
	}
	p, err := s.svc.Provider(ctx, projectID)
	if err != nil {
		return nil, err
	}
	used, err := s.svc.SearchUsers(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := &webtoolv1.GetWebSearchProviderResponse{AvailableKinds: availableKindsToProto(), UsedBy: make([]*webtoolv1.UsedBy, 0, len(used))}
	if p != nil {
		out.Provider = providerToProto(p)
	}
	for _, u := range used {
		// SECURITY: a resource-limited key sees only the agents it reaches.
		if !authz.ReachResource(ctx, orgOf(ctx), projectID, u.AgentID) {
			continue
		}
		out.UsedBy = append(out.UsedBy, &webtoolv1.UsedBy{Id: u.PublicID, Name: u.Name})
	}
	return connect.NewResponse(out), nil
}

// SetWebSearchProvider sets the project's search provider, sealing a new key.
func (s *WebToolService) SetWebSearchProvider(
	ctx context.Context, req *connect.Request[webtoolv1.SetWebSearchProviderRequest],
) (*connect.Response[webtoolv1.SetWebSearchProviderResponse], error) {
	m := req.Msg
	ctx, projectID, err := s.project(ctx, m.GetProjectId(), authz.ReachWholeProject)
	if err != nil {
		return nil, err
	}
	p, err := s.svc.SetProvider(ctx, projectID, kindOf(m.GetKind()), m.ApiKey, m.GetPricePerCallMicros())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&webtoolv1.SetWebSearchProviderResponse{Provider: providerToProto(p)}), nil
}

// DeleteWebSearchProvider removes the project's search provider; agents that use it then run without web search.
func (s *WebToolService) DeleteWebSearchProvider(
	ctx context.Context, req *connect.Request[webtoolv1.DeleteWebSearchProviderRequest],
) (*connect.Response[webtoolv1.DeleteWebSearchProviderResponse], error) {
	ctx, projectID, err := s.project(ctx, req.Msg.GetProjectId(), authz.ReachWholeProject)
	if err != nil {
		return nil, err
	}
	if err := s.svc.DeleteProvider(ctx, projectID); err != nil {
		return nil, err
	}
	return connect.NewResponse(&webtoolv1.DeleteWebSearchProviderResponse{}), nil
}

// ListHTTPSecrets lists the project's HTTP secrets, never their values.
func (s *WebToolService) ListHTTPSecrets(
	ctx context.Context, req *connect.Request[webtoolv1.ListHTTPSecretsRequest],
) (*connect.Response[webtoolv1.ListHTTPSecretsResponse], error) {
	ctx, projectID, err := s.project(ctx, req.Msg.GetProjectId(), authz.ReachProject)
	if err != nil {
		return nil, err
	}
	list, err := s.svc.Secrets(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]*webtoolv1.HTTPSecret, 0, len(list))
	for _, sec := range list {
		// SECURITY: a resource-limited key sees only the secrets it holds; host and header names are not for every caller.
		if !authz.ReachResource(ctx, sec.OrgID, sec.ProjectID, sec.ID) {
			continue
		}
		out = append(out, secretToProto(sec))
	}
	return connect.NewResponse(&webtoolv1.ListHTTPSecretsResponse{Secrets: out}), nil
}

// SetHTTPSecret stores a secret value for a host and header of the project.
func (s *WebToolService) SetHTTPSecret(
	ctx context.Context, req *connect.Request[webtoolv1.SetHTTPSecretRequest],
) (*connect.Response[webtoolv1.SetHTTPSecretResponse], error) {
	m := req.Msg
	ctx, projectID, err := s.project(ctx, m.GetProjectId(), authz.ReachWholeProject)
	if err != nil {
		return nil, err
	}
	sec, err := s.svc.SetSecret(ctx, projectID, m.GetHost(), m.GetHeader(), m.GetValue())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&webtoolv1.SetHTTPSecretResponse{Secret: secretToProto(sec)}), nil
}

// DeleteHTTPSecret removes an HTTP secret by id.
func (s *WebToolService) DeleteHTTPSecret(
	ctx context.Context, req *connect.Request[webtoolv1.DeleteHTTPSecretRequest],
) (*connect.Response[webtoolv1.DeleteHTTPSecretResponse], error) {
	r, err := s.svc.ResolveSecret(ctx, req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	// SECURITY: an unknown secret and one outside the caller's reach answer the same not-found.
	if !authz.ReachResource(ctx, r.OrgID, r.ProjectID, r.ID) {
		return nil, &NotFoundError{}
	}
	if err := s.svc.DeleteSecret(tenant.WithOrg(ctx, r.OrgID), r.ID); err != nil {
		return nil, err
	}
	return connect.NewResponse(&webtoolv1.DeleteHTTPSecretResponse{}), nil
}

func orgOf(ctx context.Context) uuid.UUID {
	tc, _ := tenant.From(ctx)
	return tc.OrgID
}

// SECURITY: an unknown or unreachable project answers the same not-found; a reached project scopes the call to its org.
func (s *WebToolService) project(
	ctx context.Context, publicID string, reach func(ctx context.Context, orgID, projectID uuid.UUID) bool,
) (context.Context, uuid.UUID, error) {
	id, orgID, ok, err := s.refs.ProjectID(ctx, publicID)
	if err != nil {
		return ctx, uuid.Nil, err
	}
	if !ok || !reach(ctx, orgID, id) {
		return ctx, uuid.Nil, &NotFoundError{}
	}
	return tenant.WithProject(tenant.WithOrg(ctx, orgID), id), id, nil
}
