package webhook

import (
	"context"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	"altalune.id/oagent/apperror"
	webhookv1 "altalune.id/oagent/gen/go/webhook/v1"
	webhookv1connect "altalune.id/oagent/gen/go/webhook/v1/webhookv1connect"
	"altalune.id/oagent/internal/kernel/authz"
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

// RetryWebhookDelivery requeues one failed delivery of an endpoint.
func (s *WebhookControlPlane) RetryWebhookDelivery(
	ctx context.Context,
	req *connect.Request[webhookv1.RetryWebhookDeliveryRequest],
) (*connect.Response[webhookv1.RetryWebhookDeliveryResponse], error) {
	ctx, id, err := scopedID(ctx, req.Msg.GetOrgId(), req.Msg.GetProjectId(), req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	deliveryID, err := uuid.Parse(req.Msg.GetDeliveryId())
	if err != nil {
		return nil, apperror.New(apperror.CodeValidation, "invalid delivery id").WithCause(err)
	}
	if err := s.webhooks.Retry(ctx, id, deliveryID); err != nil {
		return nil, err
	}
	return connect.NewResponse(&webhookv1.RetryWebhookDeliveryResponse{}), nil
}

// RetryFailedWebhookDeliveries requeues every failed delivery of an endpoint and returns the count.
func (s *WebhookControlPlane) RetryFailedWebhookDeliveries(
	ctx context.Context,
	req *connect.Request[webhookv1.RetryFailedWebhookDeliveriesRequest],
) (*connect.Response[webhookv1.RetryFailedWebhookDeliveriesResponse], error) {
	ctx, id, err := scopedID(ctx, req.Msg.GetOrgId(), req.Msg.GetProjectId(), req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	n, err := s.webhooks.RetryFailed(ctx, id)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&webhookv1.RetryFailedWebhookDeliveriesResponse{Requeued: int32(n)}), nil //nolint:gosec // G115: bounded
}

// ListWebhookDeliveries lists an endpoint's recent deliveries, newest first.
func (s *WebhookControlPlane) ListWebhookDeliveries(
	ctx context.Context,
	req *connect.Request[webhookv1.ListWebhookDeliveriesRequest],
) (*connect.Response[webhookv1.ListWebhookDeliveriesResponse], error) {
	ctx, id, err := scopedID(ctx, req.Msg.GetOrgId(), req.Msg.GetProjectId(), req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	deliveries, err := s.webhooks.Deliveries(ctx, id, int(req.Msg.GetLimit()))
	if err != nil {
		return nil, err
	}
	out := make([]*webhookv1.WebhookDelivery, 0, len(deliveries))
	for _, d := range deliveries {
		out = append(out, deliveryToProto(d))
	}
	return connect.NewResponse(&webhookv1.ListWebhookDeliveriesResponse{Deliveries: out}), nil
}

// ListWebhookDeliveryAttempts lists the attempts of one delivery to an endpoint.
func (s *WebhookControlPlane) ListWebhookDeliveryAttempts(
	ctx context.Context,
	req *connect.Request[webhookv1.ListWebhookDeliveryAttemptsRequest],
) (*connect.Response[webhookv1.ListWebhookDeliveryAttemptsResponse], error) {
	ctx, id, err := scopedID(ctx, req.Msg.GetOrgId(), req.Msg.GetProjectId(), req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	deliveryID, err := uuid.Parse(req.Msg.GetDeliveryId())
	if err != nil {
		return nil, apperror.New(apperror.CodeValidation, "invalid delivery id").WithCause(err)
	}
	attempts, err := s.webhooks.Attempts(ctx, id, deliveryID)
	if err != nil {
		return nil, err
	}
	out := make([]*webhookv1.WebhookAttempt, 0, len(attempts))
	for _, a := range attempts {
		out = append(out, attemptToProto(a))
	}
	return connect.NewResponse(&webhookv1.ListWebhookDeliveryAttemptsResponse{Attempts: out}), nil
}

// SECURITY: a project outside the caller's reach is reported as an absent webhook.
func scopeCtx(ctx context.Context, orgID, projectID string) (context.Context, error) {
	org, project, err := parseScope(orgID, projectID)
	if err != nil {
		return ctx, err
	}
	if !authz.ReachWholeProject(ctx, org, project) {
		return ctx, &NotFoundError{}
	}
	return tenant.Into(ctx, tenant.Context{OrgID: org, ProjectID: project}), nil
}

// SECURITY: an endpoint outside the caller's reach is reported as absent.
func scopedID(ctx context.Context, orgID, projectID, id string) (context.Context, uuid.UUID, error) {
	org, project, err := parseScope(orgID, projectID)
	if err != nil {
		return ctx, uuid.Nil, err
	}
	parsed, err := uuid.Parse(id)
	if err != nil {
		return ctx, uuid.Nil, apperror.New(apperror.CodeValidation, "invalid webhook id").WithCause(err)
	}
	if !authz.ReachResource(ctx, org, project, parsed) {
		return ctx, uuid.Nil, &NotFoundError{ID: parsed.String()}
	}
	return tenant.Into(ctx, tenant.Context{OrgID: org, ProjectID: project}), parsed, nil
}

func parseScope(orgID, projectID string) (org, project uuid.UUID, err error) {
	if org, err = uuid.Parse(orgID); err != nil {
		return uuid.Nil, uuid.Nil, apperror.New(apperror.CodeValidation, "invalid organization id").WithCause(err)
	}
	if project, err = uuid.Parse(projectID); err != nil {
		return uuid.Nil, uuid.Nil, apperror.New(apperror.CodeValidation, "invalid project id").WithCause(err)
	}
	return org, project, nil
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
		PublicId:    e.PublicID,
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

func deliveryToProto(d Delivery) *webhookv1.WebhookDelivery {
	return &webhookv1.WebhookDelivery{
		Id:            d.ID.String(),
		EventId:       d.EventID.String(),
		EventType:     string(d.EventType),
		Attempt:       int32(d.Attempt), //nolint:gosec // G115: bounded
		Status:        string(d.Status),
		LastError:     d.LastError,
		NextAttemptAt: timestamppb.New(d.NextAttemptAt),
		CreatedAt:     timestamppb.New(d.CreatedAt),
		Payload:       string(d.Payload),
	}
}

func attemptToProto(a Attempt) *webhookv1.WebhookAttempt {
	headers := make([]*webhookv1.WebhookResponseHeader, len(a.ResponseHeaders))
	for i, h := range a.ResponseHeaders {
		headers[i] = &webhookv1.WebhookResponseHeader{Name: h.Name, Value: h.Value}
	}
	return &webhookv1.WebhookAttempt{
		Attempt:           int32(a.Attempt),    //nolint:gosec // G115: bounded
		StatusCode:        int32(a.StatusCode), //nolint:gosec // G115: bounded
		Error:             a.Error,
		ResponseBody:      a.ResponseBody,
		ResponseTruncated: a.ResponseTruncated,
		DurationMs:        int32(a.Duration.Milliseconds()), //nolint:gosec // G115: bounded
		CreatedAt:         timestamppb.New(a.CreatedAt),
		ResponseHeaders:   headers,
	}
}
