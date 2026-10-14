// Package queue submits jobs and broadcasts over NATS JetStream.
package queue

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

type declarable interface {
	comparable
	Validate() error
	Subject() string
}

func collect[T declarable](items []T) (set map[T]struct{}, dup []string, err error) {
	set = make(map[T]struct{}, len(items))
	for _, it := range items {
		if err := it.Validate(); err != nil {
			return nil, nil, err
		}
		if _, seen := set[it]; seen {
			dup = append(dup, it.Subject())
		}
		set[it] = struct{}{}
	}
	return set, dup, nil
}

func checkWiring[T declarable](items []T, hasHandle func(i int) bool) error {
	for i, it := range items {
		if !hasHandle(i) {
			return &NilHandlerError{Subject: it.Subject()}
		}
	}
	_, dup, err := collect(items)
	if err != nil {
		return err
	}
	return wiringError(dup)
}
