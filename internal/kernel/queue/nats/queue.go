// Package nats is the queue driver on NATS JetStream: streams WORK, DLQ and BROADCAST in one account.
package nats

import (
	"context"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"

	"altalune.id/oagent/internal/kernel/tenant"
)

const (
	publishBudget     = 5 * time.Second
	publishAttempt    = time.Second
	publishRetryGap   = 200 * time.Millisecond
	workMaxAge        = 7 * 24 * time.Hour
	dlqMaxAge         = 30 * 24 * time.Hour
	broadcastMaxAge   = time.Hour
	streamMaxBytes    = 256 << 20
	broadcastMaxBytes = 64 << 20
	duplicateWindow   = 2 * time.Minute
	streamReplicas    = 1
)

func spanAttributes(ctx context.Context, p publication) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, 6)
	attrs = append(attrs,
		attribute.String("messaging.system", "nats"),
		attribute.String(p.kind, p.name),
		attribute.String("subject", p.subject),
		attribute.String("stream", p.stream),
	)
	tc, err := tenant.From(ctx)
	if err != nil {
		return attrs
	}
	return append(attrs, tenantAttributes(tc)...)
}

func tenantAttributes(tc tenant.Context) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, 2)
	attrs = append(attrs, attribute.String("org_id", tc.OrgID.String()))
	if tc.ProjectID != uuid.Nil {
		attrs = append(attrs, attribute.String("project_id", tc.ProjectID.String()))
	}
	return attrs
}
