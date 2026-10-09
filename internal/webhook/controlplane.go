package webhook

import (
	"context"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	"altalune.id/oagent/apperror"
	webhookv1 "altalune.id/oagent/gen/go/webhook/v1"
	webhookv1connect "altalune.id/oagent/gen/go/webhook/v1/webhookv1connect"
	"altalune.id/oagent/internal/kernel/events"
	"altalune.id/oagent/internal/kernel/tenant"
)

// WebhookControlPlane implements webhook.v1.WebhookService.
type WebhookControlPlane struct {
	webhooks *Service
}

// NewControlPlane binds the handler to the webhook service.
func NewControlPlane(webhooks *Service) *WebhookControlPlane {
	return &WebhookControlPlane{webhooks: webhooks}
}

var _ webhookv1connect.WebhookServiceHandler = (*WebhookControlPlane)(nil)

// CreateWebhook creates a project endpoint and returns its signing secret once.
func (s *WebhookControlPlane) CreateWebhook(
	ctx context.Context,
	req *connect.Request[webhookv1.CreateWebhookRequest],
) (*connect.Response[webhookv1.CreateWebhookResponse], error) {
	ctx, err := scopeCtx(ctx, req.Msg.GetOrgId(), req.Msg.GetProjectId())
	if err != nil {
		return nil, err
	}
	e, secret, err := s.webhooks.Create(ctx, req.Msg.GetUrl(), req.Msg.GetDescription(), toEventTypes(req.Msg.GetEventTypes()))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&webhookv1.CreateWebhookResponse{Webhook: webhookToProto(e), Secret: secret}), nil
}

// GetWebhook loads a project endpoint by id.
func (s *WebhookControlPlane) GetWebhook(
	ctx context.Context,
	req *connect.Request[webhookv1.GetWebhookRequest],
) (*connect.Response[webhookv1.GetWebhookResponse], error) {
	ctx, id, err := scopedID(ctx, req.Msg.GetOrgId(), req.Msg.GetProjectId(), req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	e, err := s.webhooks.ByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&webhookv1.GetWebhookResponse{Webhook: webhookToProto(e)}), nil
}

// ListWebhooks lists a project's endpoints, newest first.
func (s *WebhookControlPlane) ListWebhooks(
	ctx context.Context,
	req *connect.Request[webhookv1.ListWebhooksRequest],
) (*connect.Response[webhookv1.ListWebhooksResponse], error) {
	ctx, err := scopeCtx(ctx, req.Msg.GetOrgId(), req.Msg.GetProjectId())
	if err != nil {
		return nil, err
	}
	endpoints, err := s.webhooks.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*webhookv1.Webhook, 0, len(endpoints))
	for _, e := range endpoints {
		out = append(out, webhookToProto(e))
	}
	return connect.NewResponse(&webhookv1.ListWebhooksResponse{Webhooks: out}), nil
}

// UpdateWebhook replaces an endpoint's editable fields.
func (s *WebhookControlPlane) UpdateWebhook(
	ctx context.Context,
	req *connect.Request[webhookv1.UpdateWebhookRequest],
) (*connect.Response[webhookv1.UpdateWebhookResponse], error) {
	ctx, id, err := scopedID(ctx, req.Msg.GetOrgId(), req.Msg.GetProjectId(), req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	e, err := s.webhooks.Update(
		ctx, id, req.Msg.GetUrl(), req.Msg.GetDescription(), toEventTypes(req.Msg.GetEventTypes()), req.Msg.GetActive(),
	)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&webhookv1.UpdateWebhookResponse{Webhook: webhookToProto(e)}), nil
}

// DeleteWebhook removes an endpoint.
func (s *WebhookControlPlane) DeleteWebhook(
	ctx context.Context,
	req *connect.Request[webhookv1.DeleteWebhookRequest],
) (*connect.Response[webhookv1.DeleteWebhookResponse], error) {
	ctx, id, err := scopedID(ctx, req.Msg.GetOrgId(), req.Msg.GetProjectId(), req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	if err := s.webhooks.Delete(ctx, id); err != nil {
		return nil, err
	}
	return connect.NewResponse(&webhookv1.DeleteWebhookResponse{}), nil
}

// RotateWebhookSecret issues a new primary signing secret, kept old one as secondary.
func (s *WebhookControlPlane) RotateWebhookSecret(
	ctx context.Context,
	req *connect.Request[webhookv1.RotateWebhookSecretRequest],
) (*connect.Response[webhookv1.RotateWebhookSecretResponse], error) {
	ctx, id, err := scopedID(ctx, req.Msg.GetOrgId(), req.Msg.GetProjectId(), req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	secret, err := s.webhooks.Rotate(ctx, id)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&webhookv1.RotateWebhookSecretResponse{Secret: secret}), nil
}

// PingWebhook queues a webhook.ping to an endpoint.
func (s *WebhookControlPlane) PingWebhook(
	ctx context.Context,
	req *connect.Request[webhookv1.PingWebhookRequest],
) (*connect.Response[webhookv1.PingWebhookResponse], error) {
	ctx, id, err := scopedID(ctx, req.Msg.GetOrgId(), req.Msg.GetProjectId(), req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	if err := s.webhooks.SendTest(ctx, id); err != nil {
		return nil, err
	}
	return connect.NewResponse(&webhookv1.PingWebhookResponse{}), nil
}

// scopeCtx parses the org and project ids and carries them as the tenant scope.
func scopeCtx(ctx context.Context, orgID, projectID string) (context.Context, error) {
	org, err := uuid.Parse(orgID)
	if err != nil {
		return ctx, apperror.New(apperror.CodeValidation, "invalid organization id").WithCause(err)
	}
	project, err := uuid.Parse(projectID)
	if err != nil {
		return ctx, apperror.New(apperror.CodeValidation, "invalid project id").WithCause(err)
	}
	return tenant.Into(ctx, tenant.Context{OrgID: org, ProjectID: project}), nil
}

// scopedID parses a scoped endpoint id.
func scopedID(ctx context.Context, orgID, projectID, id string) (context.Context, uuid.UUID, error) {
	ctx, err := scopeCtx(ctx, orgID, projectID)
	if err != nil {
		return ctx, uuid.Nil, err
	}
	parsed, err := uuid.Parse(id)
	if err != nil {
		return ctx, uuid.Nil, apperror.New(apperror.CodeValidation, "invalid webhook id").WithCause(err)
	}
	return ctx, parsed, nil
}

func toEventTypes(names []string) []events.Type {
	out := make([]events.Type, len(names))
	for i, n := range names {
		out[i] = events.Type(n)
	}
	return out
}

func webhookToProto(e *Endpoint) *webhookv1.Webhook {
	types := make([]string, len(e.EventTypes))
	for i, t := range e.EventTypes {
		types[i] = string(t)
	}
	return &webhookv1.Webhook{
		Id:          e.ID.String(),
		OrgId:       e.OrgID.String(),
		ProjectId:   e.ProjectID.String(),
		Url:         e.URL,
		Description: e.Description,
		EventTypes:  types,
		Active:      e.Active,
		CreatedAt:   timestamppb.New(e.CreatedAt),
		UpdatedAt:   timestamppb.New(e.UpdatedAt),
	}
}
