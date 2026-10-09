package webhook

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	webhookv1 "altalune.id/oagent/gen/go/webhook/v1"
	webhookv1connect "altalune.id/oagent/gen/go/webhook/v1/webhookv1connect"
)

type fakeClient struct{ err error }

func (f *fakeClient) CreateWebhook(context.Context, *connect.Request[webhookv1.CreateWebhookRequest]) (*connect.Response[webhookv1.CreateWebhookResponse], error) {
	if f.err != nil {
		return nil, f.err
	}
	return connect.NewResponse(&webhookv1.CreateWebhookResponse{Webhook: &webhookv1.Webhook{Id: "w1"}, Secret: "whsec_x"}), nil
}

func (f *fakeClient) GetWebhook(context.Context, *connect.Request[webhookv1.GetWebhookRequest]) (*connect.Response[webhookv1.GetWebhookResponse], error) {
	if f.err != nil {
		return nil, f.err
	}
	return connect.NewResponse(&webhookv1.GetWebhookResponse{Webhook: &webhookv1.Webhook{Id: "w1", ProjectId: "p"}}), nil
}

func (f *fakeClient) ListWebhooks(context.Context, *connect.Request[webhookv1.ListWebhooksRequest]) (*connect.Response[webhookv1.ListWebhooksResponse], error) {
	if f.err != nil {
		return nil, f.err
	}
	return connect.NewResponse(&webhookv1.ListWebhooksResponse{Webhooks: []*webhookv1.Webhook{{Id: "w1", EventTypes: []string{"agent.created"}, Active: true}}}), nil
}

func (f *fakeClient) UpdateWebhook(context.Context, *connect.Request[webhookv1.UpdateWebhookRequest]) (*connect.Response[webhookv1.UpdateWebhookResponse], error) {
	if f.err != nil {
		return nil, f.err
	}
	return connect.NewResponse(&webhookv1.UpdateWebhookResponse{Webhook: &webhookv1.Webhook{Id: "w1"}}), nil
}

func (f *fakeClient) DeleteWebhook(context.Context, *connect.Request[webhookv1.DeleteWebhookRequest]) (*connect.Response[webhookv1.DeleteWebhookResponse], error) {
	if f.err != nil {
		return nil, f.err
	}
	return connect.NewResponse(&webhookv1.DeleteWebhookResponse{}), nil
}

func (f *fakeClient) RotateWebhookSecret(context.Context, *connect.Request[webhookv1.RotateWebhookSecretRequest]) (*connect.Response[webhookv1.RotateWebhookSecretResponse], error) {
	if f.err != nil {
		return nil, f.err
	}
	return connect.NewResponse(&webhookv1.RotateWebhookSecretResponse{Secret: "whsec_new"}), nil
}

func (f *fakeClient) PingWebhook(context.Context, *connect.Request[webhookv1.PingWebhookRequest]) (*connect.Response[webhookv1.PingWebhookResponse], error) {
	if f.err != nil {
		return nil, f.err
	}
	return connect.NewResponse(&webhookv1.PingWebhookResponse{}), nil
}

func (f *fakeClient) RetryWebhookDelivery(context.Context, *connect.Request[webhookv1.RetryWebhookDeliveryRequest]) (*connect.Response[webhookv1.RetryWebhookDeliveryResponse], error) {
	if f.err != nil {
		return nil, f.err
	}
	return connect.NewResponse(&webhookv1.RetryWebhookDeliveryResponse{}), nil
}

func (f *fakeClient) RetryFailedWebhookDeliveries(context.Context, *connect.Request[webhookv1.RetryFailedWebhookDeliveriesRequest]) (*connect.Response[webhookv1.RetryFailedWebhookDeliveriesResponse], error) {
	if f.err != nil {
		return nil, f.err
	}
	return connect.NewResponse(&webhookv1.RetryFailedWebhookDeliveriesResponse{Requeued: 2}), nil
}

func (f *fakeClient) ListWebhookDeliveries(context.Context, *connect.Request[webhookv1.ListWebhookDeliveriesRequest]) (*connect.Response[webhookv1.ListWebhookDeliveriesResponse], error) {
	if f.err != nil {
		return nil, f.err
	}
	return connect.NewResponse(&webhookv1.ListWebhookDeliveriesResponse{
		Deliveries: []*webhookv1.WebhookDelivery{{Id: "d1", EventType: "agent.created", Status: "delivered", Attempt: 1, CreatedAt: timestamppb.Now()}},
	}), nil
}

func (f *fakeClient) ListWebhookDeliveryAttempts(context.Context, *connect.Request[webhookv1.ListWebhookDeliveryAttemptsRequest]) (*connect.Response[webhookv1.ListWebhookDeliveryAttemptsResponse], error) {
	if f.err != nil {
		return nil, f.err
	}
	return connect.NewResponse(&webhookv1.ListWebhookDeliveryAttemptsResponse{
		Attempts: []*webhookv1.WebhookAttempt{{Attempt: 1, StatusCode: 200, DurationMs: 3, CreatedAt: timestamppb.Now()}},
	}), nil
}

var _ webhookv1connect.WebhookServiceClient = (*fakeClient)(nil)

func runCmd(t *testing.T, client *fakeClient, args ...string) error {
	t.Helper()
	cmd := NewCmd(func(*cobra.Command) (webhookv1connect.WebhookServiceClient, error) { return client, nil })
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)
	return cmd.ExecuteContext(context.Background())
}

func TestCLICommands(t *testing.T) {
	t.Setenv("OAGENT_OUTPUT", "text")
	c := &fakeClient{}
	require.NoError(t, runCmd(t, c, "create", "--org-id", "o", "--project-id", "p", "--endpoint-url", "https://e.com", "--event", "agent.created"))
	require.NoError(t, runCmd(t, c, "get", "w1", "--org-id", "o", "--project-id", "p"))
	require.NoError(t, runCmd(t, c, "list", "--org-id", "o", "--project-id", "p"))
	require.NoError(t, runCmd(t, c, "update", "w1", "--org-id", "o", "--project-id", "p", "--endpoint-url", "https://e.com", "--event", "agent.updated"))
	require.NoError(t, runCmd(t, c, "delete", "w1", "--org-id", "o", "--project-id", "p"))
	require.NoError(t, runCmd(t, c, "rotate", "w1", "--org-id", "o", "--project-id", "p"))
	require.NoError(t, runCmd(t, c, "ping", "w1", "--org-id", "o", "--project-id", "p"))
	require.NoError(t, runCmd(t, c, "retry", "w1", "d1", "--org-id", "o", "--project-id", "p"))
	require.NoError(t, runCmd(t, c, "retry-failed", "w1", "--org-id", "o", "--project-id", "p"))
	require.NoError(t, runCmd(t, c, "deliveries", "w1", "--org-id", "o", "--project-id", "p"))
	require.NoError(t, runCmd(t, c, "attempts", "w1", "d1", "--org-id", "o", "--project-id", "p"))
}

func TestCLIJSON(t *testing.T) {
	t.Setenv("OAGENT_OUTPUT", "json")
	c := &fakeClient{}
	require.NoError(t, runCmd(t, c, "create", "--org-id", "o", "--project-id", "p", "--endpoint-url", "https://e.com", "--event", "agent.created"))
	require.NoError(t, runCmd(t, c, "get", "w1", "--org-id", "o", "--project-id", "p"))
	require.NoError(t, runCmd(t, c, "list", "--org-id", "o", "--project-id", "p"))
	require.NoError(t, runCmd(t, c, "update", "w1", "--org-id", "o", "--project-id", "p", "--endpoint-url", "https://e.com", "--event", "agent.updated"))
}

func TestCLIErrors(t *testing.T) {
	c := &fakeClient{err: errors.New("boom")}
	assert.Error(t, runCmd(t, c, "create", "--org-id", "o", "--project-id", "p", "--endpoint-url", "https://e.com", "--event", "agent.created"))
	assert.Error(t, runCmd(t, c, "get", "w1", "--org-id", "o", "--project-id", "p"))
	assert.Error(t, runCmd(t, c, "list", "--org-id", "o", "--project-id", "p"))
	assert.Error(t, runCmd(t, c, "update", "w1", "--org-id", "o", "--project-id", "p", "--endpoint-url", "https://e.com", "--event", "agent.updated"))
	assert.Error(t, runCmd(t, c, "delete", "w1", "--org-id", "o", "--project-id", "p"))
	assert.Error(t, runCmd(t, c, "rotate", "w1", "--org-id", "o", "--project-id", "p"))
	assert.Error(t, runCmd(t, c, "ping", "w1", "--org-id", "o", "--project-id", "p"))
	assert.Error(t, runCmd(t, c, "retry", "w1", "d1", "--org-id", "o", "--project-id", "p"))
	assert.Error(t, runCmd(t, c, "retry-failed", "w1", "--org-id", "o", "--project-id", "p"))
	assert.Error(t, runCmd(t, c, "deliveries", "w1", "--org-id", "o", "--project-id", "p"))
	assert.Error(t, runCmd(t, c, "attempts", "w1", "d1", "--org-id", "o", "--project-id", "p"))
}
