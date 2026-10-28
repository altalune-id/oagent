package boot

import (
	"context"
	"log/slog"
	"time"

	"altalune.id/oagent/httpclient"
	"altalune.id/oagent/internal/kernel/outbox"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/webhook"
)

const webhookDeliveryTimeout = 10 * time.Second

func buildOutboxWorker(svcs *Services, log *slog.Logger) *outbox.Worker {
	// Webhook endpoints are frequently internal in self-hosted deployments, so the SSRF filter is off here.
	client := httpclient.New(httpclient.WithAllowPrivateHosts(true), httpclient.WithTimeout(webhookDeliveryTimeout))
	deliverer := webhook.NewDeliverer(svcs.WebhookStore, svcs.Sealer, client, log)
	return outbox.NewWorker(svcs.Outbox, deliverer, outboxTenants{orgs: svcs.Orgs}, log, outbox.WorkerOpts{})
}

type outboxTenants struct{ orgs *org.Service }

func (t outboxTenants) Each(ctx context.Context, fn func(context.Context, string) error) error {
	ids, err := t.orgs.IDs(ctx)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := fn(tenant.Into(ctx, tenant.Context{OrgID: id}), id.String()); err != nil {
			return err
		}
	}
	return nil
}
