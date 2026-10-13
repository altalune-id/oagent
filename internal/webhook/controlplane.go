package webhook

import (
	"context"
	"slices"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	webhookv1 "altalune.id/oagent/gen/go/webhook/v1"
	webhookv1connect "altalune.id/oagent/gen/go/webhook/v1/webhookv1connect"
	"altalune.id/oagent/internal/kernel/authz"
	"altalune.id/oagent/internal/kernel/events"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/scope"
)

// WebhookControlPlane implements webhook.v1.WebhookService; every id it takes and returns is a public id.
type WebhookControlPlane struct {
	webhooks *Service
	refs     Refs
}

// NewControlPlane binds the handler to the webhook service and the org and project public id lookups.
func NewControlPlane(webhooks *Service, refs Refs) *WebhookControlPlane {
	return &WebhookControlPlane{webhooks: webhooks, refs: refs}
}

var _ webhookv1connect.WebhookServiceHandler = (*WebhookControlPlane)(nil)

// CreateWebhook creates a project endpoint and returns its signing secret once.
func (s *WebhookControlPlane) CreateWebhook(
	ctx context.Context,
	req *connect.Request[webhookv1.CreateWebhookRequest],
) (*connect.Response[webhookv1.CreateWebhookResponse], error) {
	ctx, err := s.scopeCtx(ctx, req.Msg.GetOrgId(), req.Msg.GetProjectId())
	if err != nil {
		return nil, err
	}
	types := toEventTypes(req.Msg.GetEventTypes())
	if err := requireOrgEventsReach(ctx, types); err != nil {
		return nil, err
	}
	e, secret, err := s.webhooks.Create(ctx, req.Msg.GetUrl(), req.Msg.GetDescription(), types)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&webhookv1.CreateWebhookResponse{
		Webhook: webhookToProto(e, req.Msg.GetOrgId(), req.Msg.GetProjectId()), Secret: secret,
	}), nil
}

// GetWebhook loads a project endpoint by id.
func (s *WebhookControlPlane) GetWebhook(
	ctx context.Context,
	req *connect.Request[webhookv1.GetWebhookRequest],
) (*connect.Response[webhookv1.GetWebhookResponse], error) {
	ctx, id, err := s.scopedID(ctx, req.Msg.GetOrgId(), req.Msg.GetProjectId(), req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	e, err := s.webhooks.ByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&webhookv1.GetWebhookResponse{Webhook: webhookToProto(e, req.Msg.GetOrgId(), req.Msg.GetProjectId())}), nil
}

// ListWebhooks lists a project's endpoints, newest first.
func (s *WebhookControlPlane) ListWebhooks(
	ctx context.Context,
	req *connect.Request[webhookv1.ListWebhooksRequest],
) (*connect.Response[webhookv1.ListWebhooksResponse], error) {
	ctx, err := s.scopeCtx(ctx, req.Msg.GetOrgId(), req.Msg.GetProjectId())
	if err != nil {
		return nil, err
	}
	endpoints, err := s.webhooks.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*webhookv1.Webhook, 0, len(endpoints))
	for _, e := range endpoints {
		out = append(out, webhookToProto(e, req.Msg.GetOrgId(), req.Msg.GetProjectId()))
	}
	return connect.NewResponse(&webhookv1.ListWebhooksResponse{Webhooks: out}), nil
}

// UpdateWebhook replaces an endpoint's editable fields.
func (s *WebhookControlPlane) UpdateWebhook(
	ctx context.Context,
	req *connect.Request[webhookv1.UpdateWebhookRequest],
) (*connect.Response[webhookv1.UpdateWebhookResponse], error) {
	ctx, id, err := s.scopedID(ctx, req.Msg.GetOrgId(), req.Msg.GetProjectId(), req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	types := toEventTypes(req.Msg.GetEventTypes())
	if err := requireOrgEventsReach(ctx, types); err != nil {
		return nil, err
	}
	e, err := s.webhooks.Update(ctx, id, req.Msg.GetUrl(), req.Msg.GetDescription(), types, req.Msg.GetActive())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&webhookv1.UpdateWebhookResponse{Webhook: webhookToProto(e, req.Msg.GetOrgId(), req.Msg.GetProjectId())}), nil
}

// DeleteWebhook removes an endpoint.
func (s *WebhookControlPlane) DeleteWebhook(
	ctx context.Context,
	req *connect.Request[webhookv1.DeleteWebhookRequest],
) (*connect.Response[webhookv1.DeleteWebhookResponse], error) {
	ctx, id, err := s.scopedID(ctx, req.Msg.GetOrgId(), req.Msg.GetProjectId(), req.Msg.GetId())
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
	ctx, id, err := s.scopedID(ctx, req.Msg.GetOrgId(), req.Msg.GetProjectId(), req.Msg.GetId())
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
	ctx, id, err := s.scopedID(ctx, req.Msg.GetOrgId(), req.Msg.GetProjectId(), req.Msg.GetId())
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
	ctx, id, err := s.scopedID(ctx, req.Msg.GetOrgId(), req.Msg.GetProjectId(), req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	deliveryID, err := s.webhooks.DeliveryIDByPublicID(ctx, id, req.Msg.GetDeliveryId())
	if err != nil {
		return nil, err
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
	ctx, id, err := s.scopedID(ctx, req.Msg.GetOrgId(), req.Msg.GetProjectId(), req.Msg.GetId())
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
	ctx, id, err := s.scopedID(ctx, req.Msg.GetOrgId(), req.Msg.GetProjectId(), req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	deliveries, err := s.webhooks.Deliveries(ctx, id, int(req.Msg.GetLimit()))
	if err != nil {
		return nil, err
	}
	seesOrg := seesOrgEvents(ctx)
	out := make([]*webhookv1.WebhookDelivery, 0, len(deliveries))
	for _, d := range deliveries {
		pd := deliveryToProto(d)
		if !seesOrg && events.IsOrgWide(d.EventType) {
			pd.Payload = ""
		}
		out = append(out, pd)
	}
	return connect.NewResponse(&webhookv1.ListWebhookDeliveriesResponse{Deliveries: out}), nil
}

// ListWebhookDeliveryAttempts lists the attempts of one delivery to an endpoint.
func (s *WebhookControlPlane) ListWebhookDeliveryAttempts(
	ctx context.Context,
	req *connect.Request[webhookv1.ListWebhookDeliveryAttemptsRequest],
) (*connect.Response[webhookv1.ListWebhookDeliveryAttemptsResponse], error) {
	ctx, id, err := s.scopedID(ctx, req.Msg.GetOrgId(), req.Msg.GetProjectId(), req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	deliveryID, err := s.webhooks.DeliveryIDByPublicID(ctx, id, req.Msg.GetDeliveryId())
	if err != nil {
		return nil, err
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

// SECURITY: an unknown project and a project outside the caller's reach are reported as an absent webhook.
func (s *WebhookControlPlane) scopeCtx(ctx context.Context, orgPublicID, projectPublicID string) (context.Context, error) {
	orgID, ok, err := s.refs.OrgID(ctx, orgPublicID)
	if err != nil {
		return ctx, err
	}
	projectID, projectOrg, projectOK, err := s.refs.ProjectID(ctx, projectPublicID)
	if err != nil {
		return ctx, err
	}
	if !ok || !projectOK || projectOrg != orgID || !authz.ReachWholeProject(ctx, orgID, projectID) {
		return ctx, &NotFoundError{}
	}
	return tenant.Into(ctx, tenant.Context{OrgID: orgID, ProjectID: projectID}), nil
}

// SECURITY: an unknown endpoint and an endpoint outside the caller's reach answer the same not-found.
func (s *WebhookControlPlane) scopedID(
	ctx context.Context, orgPublicID, projectPublicID, publicID string,
) (context.Context, uuid.UUID, error) {
	orgID, ok, err := s.refs.OrgID(ctx, orgPublicID)
	if err != nil {
		return ctx, uuid.Nil, err
	}
	projectID, projectOrg, projectOK, err := s.refs.ProjectID(ctx, projectPublicID)
	if err != nil {
		return ctx, uuid.Nil, err
	}
	if !ok || !projectOK || projectOrg != orgID {
		return ctx, uuid.Nil, &NotFoundError{}
	}
	ctx = tenant.Into(ctx, tenant.Context{OrgID: orgID, ProjectID: projectID})
	id, err := s.webhooks.IDByPublicID(ctx, publicID)
	if err != nil {
		return ctx, uuid.Nil, err
	}
	if !authz.ReachResource(ctx, orgID, projectID, id) {
		return ctx, uuid.Nil, &NotFoundError{}
	}
	return ctx, id, nil
}

// SECURITY: org events carry every invitee's and member's email; only a caller reaching the whole org with member:read subscribes.
func requireOrgEventsReach(ctx context.Context, types []events.Type) error {
	if !slices.ContainsFunc(types, events.IsOrgWide) || seesOrgEvents(ctx) {
		return nil
	}
	if tc, err := tenant.From(ctx); err != nil || !authz.ReachWholeOrg(ctx, tc.OrgID) {
		return &authz.ForbiddenError{}
	}
	return &authz.ForbiddenError{Permission: scope.MemberRead}
}

func seesOrgEvents(ctx context.Context) bool {
	tc, err := tenant.From(ctx)
	return err == nil && authz.ReachWholeOrg(ctx, tc.OrgID) && authz.Allowed(session.PrincipalFrom(ctx), scope.MemberRead)
}

func toEventTypes(names []string) []events.Type {
	out := make([]events.Type, len(names))
	for i, n := range names {
		out[i] = events.Type(n)
	}
	return out
}

func webhookToProto(e *Endpoint, orgPublicID, projectPublicID string) *webhookv1.Webhook {
	types := make([]string, len(e.EventTypes))
	for i, t := range e.EventTypes {
		types[i] = string(t)
	}
	return &webhookv1.Webhook{
		Id:          e.PublicID,
		OrgId:       orgPublicID,
		ProjectId:   projectPublicID,
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
		Id:            d.PublicID,
		EventId:       d.EventPublicID,
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
