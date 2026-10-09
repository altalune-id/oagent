package webhook

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	webhookv1 "altalune.id/oagent/gen/go/webhook/v1"
	"altalune.id/oagent/internal/kernel/events"
	"altalune.id/oagent/internal/kernel/outbox"
)

func controlPlane(t *testing.T, store *fakeStore) (*WebhookControlPlane, uuid.UUID, uuid.UUID) {
	t.Helper()
	svc := testService(t, store, &fakeOutbox{})
	return NewControlPlane(svc), uuid.New(), uuid.New()
}

func TestControlPlaneCreate(t *testing.T) {
	cp, org, project := controlPlane(t, &fakeStore{})
	resp, err := cp.CreateWebhook(context.Background(), connect.NewRequest(&webhookv1.CreateWebhookRequest{
		OrgId: org.String(), ProjectId: project.String(), Url: "https://e.com", EventTypes: []string{string(events.AgentCreated)},
	}))
	require.NoError(t, err)
	assert.NotEmpty(t, resp.Msg.GetSecret())
	assert.Equal(t, org.String(), resp.Msg.GetWebhook().GetOrgId())
}

func TestControlPlaneValidationError(t *testing.T) {
	cp, _, _ := controlPlane(t, &fakeStore{})
	_, err := cp.CreateWebhook(context.Background(), connect.NewRequest(&webhookv1.CreateWebhookRequest{
		OrgId: "bad", ProjectId: "bad", Url: "https://e.com", EventTypes: []string{string(events.AgentCreated)},
	}))
	assert.Error(t, err)
}

func TestControlPlaneGetList(t *testing.T) {
	org, project := uuid.New(), uuid.New()
	e := &Endpoint{ID: uuid.New(), OrgID: org, ProjectID: project, URL: "https://e.com", Active: true, EventTypes: []events.Type{events.AgentCreated}}
	cp, _, _ := controlPlane(t, &fakeStore{endpoints: []*Endpoint{e}})

	got, err := cp.GetWebhook(context.Background(), connect.NewRequest(&webhookv1.GetWebhookRequest{OrgId: org.String(), ProjectId: project.String(), Id: e.ID.String()}))
	require.NoError(t, err)
	assert.Equal(t, e.ID.String(), got.Msg.GetWebhook().GetId())

	list, err := cp.ListWebhooks(context.Background(), connect.NewRequest(&webhookv1.ListWebhooksRequest{OrgId: org.String(), ProjectId: project.String()}))
	require.NoError(t, err)
	assert.Len(t, list.Msg.GetWebhooks(), 1)

	_, err = cp.GetWebhook(context.Background(), connect.NewRequest(&webhookv1.GetWebhookRequest{OrgId: org.String(), ProjectId: project.String(), Id: "bad"}))
	assert.Error(t, err)
}

func TestControlPlaneUpdateDelete(t *testing.T) {
	org, project := uuid.New(), uuid.New()
	e := &Endpoint{ID: uuid.New(), OrgID: org, ProjectID: project, URL: "https://e.com", Active: true, EventTypes: []events.Type{events.AgentCreated}}
	cp, _, _ := controlPlane(t, &fakeStore{endpoints: []*Endpoint{e}})

	upd, err := cp.UpdateWebhook(context.Background(), connect.NewRequest(&webhookv1.UpdateWebhookRequest{
		OrgId: org.String(), ProjectId: project.String(), Id: e.ID.String(), Url: "https://e2.com", EventTypes: []string{string(events.AgentUpdated)}, Active: true,
	}))
	require.NoError(t, err)
	assert.Equal(t, "https://e2.com", upd.Msg.GetWebhook().GetUrl())

	_, err = cp.DeleteWebhook(context.Background(), connect.NewRequest(&webhookv1.DeleteWebhookRequest{OrgId: org.String(), ProjectId: project.String(), Id: e.ID.String()}))
	require.NoError(t, err)
}

func TestControlPlaneRotatePing(t *testing.T) {
	org, project := uuid.New(), uuid.New()
	e := &Endpoint{ID: uuid.New(), OrgID: org, ProjectID: project, Active: true, EventTypes: []events.Type{events.AgentCreated}}
	cp, _, _ := controlPlane(t, &fakeStore{endpoints: []*Endpoint{e}})

	rot, err := cp.RotateWebhookSecret(context.Background(), connect.NewRequest(&webhookv1.RotateWebhookSecretRequest{OrgId: org.String(), ProjectId: project.String(), Id: e.ID.String()}))
	require.NoError(t, err)
	assert.NotEmpty(t, rot.Msg.GetSecret())

	_, err = cp.PingWebhook(context.Background(), connect.NewRequest(&webhookv1.PingWebhookRequest{OrgId: org.String(), ProjectId: project.String(), Id: e.ID.String()}))
	require.NoError(t, err)
}

func TestControlPlaneErrors(t *testing.T) {
	cp, org, project := controlPlane(t, &fakeStore{})
	ctx := context.Background()
	id := uuid.New().String()

	_, err := cp.GetWebhook(ctx, connect.NewRequest(&webhookv1.GetWebhookRequest{OrgId: org.String(), ProjectId: project.String(), Id: id}))
	assert.Error(t, err)
	_, err = cp.UpdateWebhook(ctx, connect.NewRequest(&webhookv1.UpdateWebhookRequest{OrgId: org.String(), ProjectId: project.String(), Id: id, Url: "https://e.com", EventTypes: []string{"agent.created"}}))
	assert.Error(t, err)
	_, err = cp.DeleteWebhook(ctx, connect.NewRequest(&webhookv1.DeleteWebhookRequest{OrgId: org.String(), ProjectId: project.String(), Id: id}))
	assert.Error(t, err)
	_, err = cp.RotateWebhookSecret(ctx, connect.NewRequest(&webhookv1.RotateWebhookSecretRequest{OrgId: org.String(), ProjectId: project.String(), Id: id}))
	assert.Error(t, err)
	_, err = cp.PingWebhook(ctx, connect.NewRequest(&webhookv1.PingWebhookRequest{OrgId: org.String(), ProjectId: project.String(), Id: id}))
	assert.Error(t, err)

	cp2 := NewControlPlane(testService(t, &fakeStore{listErr: errBoom}, &fakeOutbox{}))
	_, err = cp2.ListWebhooks(ctx, connect.NewRequest(&webhookv1.ListWebhooksRequest{OrgId: org.String(), ProjectId: project.String()}))
	assert.Error(t, err)
}

func TestControlPlaneBadProject(t *testing.T) {
	cp, org, _ := controlPlane(t, &fakeStore{})
	_, err := cp.ListWebhooks(context.Background(), connect.NewRequest(&webhookv1.ListWebhooksRequest{OrgId: org.String(), ProjectId: "bad"}))
	assert.Error(t, err)
}

func TestControlPlaneDeliveryLogs(t *testing.T) {
	org, project := uuid.New(), uuid.New()
	e := &Endpoint{ID: uuid.New(), OrgID: org, ProjectID: project}
	ob := &fakeOutbox{entries: []outbox.Entry{{ID: uuid.New(), Payload: []byte(`{"type":"agent.created"}`)}}}
	store := &fakeStore{endpoints: []*Endpoint{e}, attempts: []Attempt{{Attempt: 1, StatusCode: 200}}}
	cp := NewControlPlane(testService(t, store, ob))
	ctx := context.Background()

	resp, err := cp.ListWebhookDeliveries(ctx, connect.NewRequest(&webhookv1.ListWebhookDeliveriesRequest{
		OrgId: org.String(), ProjectId: project.String(), Id: e.ID.String(),
	}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.GetDeliveries(), 1)
	assert.Equal(t, string(events.AgentCreated), resp.Msg.GetDeliveries()[0].GetEventType())

	ar, err := cp.ListWebhookDeliveryAttempts(ctx, connect.NewRequest(&webhookv1.ListWebhookDeliveryAttemptsRequest{
		OrgId: org.String(), ProjectId: project.String(), Id: e.ID.String(), DeliveryId: uuid.New().String(),
	}))
	require.NoError(t, err)
	require.Len(t, ar.Msg.GetAttempts(), 1)

	_, err = cp.ListWebhookDeliveryAttempts(ctx, connect.NewRequest(&webhookv1.ListWebhookDeliveryAttemptsRequest{
		OrgId: org.String(), ProjectId: project.String(), Id: e.ID.String(), DeliveryId: "bad",
	}))
	assert.Error(t, err)
}
