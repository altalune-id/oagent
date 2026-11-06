package modelprovider

import (
	"context"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	modelproviderv1 "altalune.id/oagent/gen/go/modelprovider/v1"
	modelproviderv1connect "altalune.id/oagent/gen/go/modelprovider/v1/modelproviderv1connect"
	"altalune.id/oagent/internal/kernel/authz"
	"altalune.id/oagent/internal/kernel/tenant"
)

// ModelProviderService implements modelprovider.v1.ModelProviderService; every id it takes and returns is a public id.
type ModelProviderService struct {
	providers *Service
	refs      Refs
}

// NewControlPlane binds the handler to the provider service and the project public id lookups.
func NewControlPlane(providers *Service, refs Refs) *ModelProviderService {
	return &ModelProviderService{providers: providers, refs: refs}
}

var _ modelproviderv1connect.ModelProviderServiceHandler = (*ModelProviderService)(nil)

// CreateModelProvider creates a provider in a project and seals its API key.
func (s *ModelProviderService) CreateModelProvider(
	ctx context.Context,
	req *connect.Request[modelproviderv1.CreateModelProviderRequest],
) (*connect.Response[modelproviderv1.CreateModelProviderResponse], error) {
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
		return nil, &NotFoundError{}
	}
	p, err := s.providers.Create(ctx, projectID, inputFromProto(m.GetName(), m.GetKind(), m.GetBaseUrl(), m.GetModels()), m.GetApiKey())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&modelproviderv1.CreateModelProviderResponse{
		ModelProvider: providerToProto(p, m.GetOrgId(), m.GetProjectId()),
	}), nil
}

// GetModelProvider loads a provider by id.
func (s *ModelProviderService) GetModelProvider(
	ctx context.Context,
	req *connect.Request[modelproviderv1.GetModelProviderRequest],
) (*connect.Response[modelproviderv1.GetModelProviderResponse], error) {
	ctx, p, err := s.reachable(ctx, req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	out, err := s.toProto(ctx, p)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&modelproviderv1.GetModelProviderResponse{ModelProvider: out}), nil
}

// ListModelProviders lists the project's providers the caller reaches, oldest first.
func (s *ModelProviderService) ListModelProviders(
	ctx context.Context,
	req *connect.Request[modelproviderv1.ListModelProvidersRequest],
) (*connect.Response[modelproviderv1.ListModelProvidersResponse], error) {
	ctx, projectID, err := s.project(ctx, req.Msg.GetProjectId(), authz.ReachProject)
	if err != nil {
		return nil, err
	}
	ps, err := s.providers.List(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]*modelproviderv1.ModelProvider, 0, len(ps))
	orgPublicID := ""
	for _, p := range ps {
		if !authz.ReachResource(ctx, p.OrgID, p.ProjectID, p.ID) {
			continue
		}
		if orgPublicID == "" {
			if orgPublicID, err = s.refs.OrgPublicID(ctx, p.OrgID); err != nil {
				return nil, err
			}
		}
		out = append(out, providerToProto(p, orgPublicID, req.Msg.GetProjectId()))
	}
	return connect.NewResponse(&modelproviderv1.ListModelProvidersResponse{ModelProviders: out}), nil
}

// UpdateModelProvider replaces a provider's editable fields; an unset api_key keeps the stored key.
func (s *ModelProviderService) UpdateModelProvider(
	ctx context.Context,
	req *connect.Request[modelproviderv1.UpdateModelProviderRequest],
) (*connect.Response[modelproviderv1.UpdateModelProviderResponse], error) {
	m := req.Msg
	ctx, p, err := s.reachable(ctx, m.GetId())
	if err != nil {
		return nil, err
	}
	p, err = s.providers.Update(ctx, p.ID, inputFromProto(m.GetName(), m.GetKind(), m.GetBaseUrl(), m.GetModels()), m.ApiKey)
	if err != nil {
		return nil, err
	}
	out, err := s.toProto(ctx, p)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&modelproviderv1.UpdateModelProviderResponse{ModelProvider: out}), nil
}

// DeleteModelProvider deletes a provider no published agent version uses.
func (s *ModelProviderService) DeleteModelProvider(
	ctx context.Context,
	req *connect.Request[modelproviderv1.DeleteModelProviderRequest],
) (*connect.Response[modelproviderv1.DeleteModelProviderResponse], error) {
	ctx, p, err := s.reachable(ctx, req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	if err := s.providers.Delete(ctx, p.ID); err != nil {
		return nil, err
	}
	return connect.NewResponse(&modelproviderv1.DeleteModelProviderResponse{}), nil
}

// SECURITY: unknown and out-of-reach providers answer the same not-found; a reached provider scopes the call to its org.
func (s *ModelProviderService) reachable(ctx context.Context, publicID string) (context.Context, *Provider, error) {
	r, err := s.providers.Resolve(ctx, publicID)
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
	p, err := s.providers.Get(ctx, r.ID)
	if IsNotFoundError(err) {
		return ctx, nil, &NotFoundError{}
	}
	return ctx, p, err
}

// SECURITY: an unknown or unreachable project is reported as an absent provider; a reached project scopes the call to its org.
func (s *ModelProviderService) project(
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

func (s *ModelProviderService) toProto(ctx context.Context, p *Provider) (*modelproviderv1.ModelProvider, error) {
	orgPub, err := s.refs.OrgPublicID(ctx, p.OrgID)
	if err != nil {
		return nil, err
	}
	projectPub, err := s.refs.ProjectPublicID(ctx, p.ProjectID)
	if err != nil {
		return nil, err
	}
	return providerToProto(p, orgPub, projectPub), nil
}

func inputFromProto(name string, kind modelproviderv1.ModelProviderKind, baseURL string, models []*modelproviderv1.Model) Input {
	out := Input{Name: name, Kind: kindFromProto(kind), BaseURL: baseURL, Models: make([]Model, 0, len(models))}
	for _, m := range models {
		model := Model{ID: m.GetId(), Vision: m.GetVision()}
		if pr := m.GetPrices(); pr != nil {
			model.Prices = &Prices{
				InputMicros: pr.GetInputMicros(), OutputMicros: pr.GetOutputMicros(),
				CacheReadMicros: pr.GetCacheReadMicros(), CacheWriteMicros: pr.GetCacheWriteMicros(),
			}
		}
		out.Models = append(out.Models, model)
	}
	return out
}

func kindFromProto(k modelproviderv1.ModelProviderKind) Kind {
	if k == modelproviderv1.ModelProviderKind_MODEL_PROVIDER_KIND_OPENAI_COMPATIBLE {
		return KindOpenAICompatible
	}
	return ""
}

func kindToProto(k Kind) modelproviderv1.ModelProviderKind {
	if k == KindOpenAICompatible {
		return modelproviderv1.ModelProviderKind_MODEL_PROVIDER_KIND_OPENAI_COMPATIBLE
	}
	return modelproviderv1.ModelProviderKind_MODEL_PROVIDER_KIND_UNSPECIFIED
}

// SECURITY: the sealed key never leaves the server; the wire carries only whether one is set.
func providerToProto(p *Provider, orgPublicID, projectPublicID string) *modelproviderv1.ModelProvider {
	models := make([]*modelproviderv1.Model, 0, len(p.Models))
	for _, m := range p.Models {
		out := &modelproviderv1.Model{Id: m.ID, Vision: m.Vision}
		if m.Prices != nil {
			out.Prices = &modelproviderv1.ModelPrices{
				InputMicros: m.Prices.InputMicros, OutputMicros: m.Prices.OutputMicros,
				CacheReadMicros: m.Prices.CacheReadMicros, CacheWriteMicros: m.Prices.CacheWriteMicros,
			}
		}
		models = append(models, out)
	}
	return &modelproviderv1.ModelProvider{
		Id:        p.PublicID,
		OrgId:     orgPublicID,
		ProjectId: projectPublicID,
		Name:      p.Name,
		Kind:      kindToProto(p.Kind),
		BaseUrl:   p.BaseURL,
		ApiKeySet: len(p.APIKeySealed) > 0,
		Models:    models,
		CreatedAt: timestamppb.New(p.CreatedAt),
		UpdatedAt: timestamppb.New(p.UpdatedAt),
	}
}
