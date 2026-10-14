package nats

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	natsgo "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.opentelemetry.io/otel/propagation"

	"altalune.id/oagent/internal/kernel/tenant"
)

const (
	headerMsgID            = jetstream.MsgIDHeader
	headerJob              = "Oagent-Job"
	headerJobVersion       = "Oagent-Job-Version"
	headerBroadcast        = "Oagent-Broadcast"
	headerBroadcastVersion = "Oagent-Broadcast-Version"
	headerCreatedAt        = "Oagent-Created-At"
	headerOrgID            = "Oagent-Org-Id"
	headerProjectID        = "Oagent-Project-Id"
	headerUserID           = "Oagent-User-Id"
	headerDlqReason        = "Oagent-Dlq-Reason"
	headerDlqError         = "Oagent-Dlq-Error"
	headerDlqAttempts      = "Oagent-Dlq-Attempts"
	headerDlqStreamSeq     = "Oagent-Dlq-Stream-Seq"
)

type natsCarrier natsgo.Header

var _ propagation.TextMapCarrier = natsCarrier(nil)

func (c natsCarrier) Get(key string) string { return natsgo.Header(c).Get(key) }

func (c natsCarrier) Set(key, value string) { natsgo.Header(c).Set(key, value) }

func (c natsCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}

func injectTrace(ctx context.Context, h natsgo.Header) {
	propagation.TraceContext{}.Inject(ctx, natsCarrier(h))
}

func extractTrace(ctx context.Context, h natsgo.Header) context.Context {
	return propagation.TraceContext{}.Extract(ctx, natsCarrier(h))
}

func setTenant(h natsgo.Header, tc tenant.Context) {
	setID(h, headerOrgID, tc.OrgID)
	setID(h, headerProjectID, tc.ProjectID)
	setID(h, headerUserID, tc.UserID)
}

func setID(h natsgo.Header, key string, id uuid.UUID) {
	if id == uuid.Nil {
		return
	}
	h.Set(key, id.String())
}

// SECURITY: tenant headers are trusted, so anyone who can publish to the streams acts as any tenant; keep the NATS token private.
func tenantFrom(h natsgo.Header) (tenant.Context, bool, error) {
	if h.Get(headerOrgID) == "" {
		if h.Get(headerProjectID) != "" || h.Get(headerUserID) != "" {
			return tenant.Context{}, false, fmt.Errorf("queue: header %s is missing beside a project or user id", headerOrgID)
		}
		return tenant.Context{}, false, nil
	}
	var tc tenant.Context
	fields := []struct {
		key string
		dst *uuid.UUID
	}{{headerOrgID, &tc.OrgID}, {headerProjectID, &tc.ProjectID}, {headerUserID, &tc.UserID}}
	for _, f := range fields {
		if err := parseID(h, f.key, f.dst); err != nil {
			return tenant.Context{}, false, err
		}
	}
	return tc, true, nil
}

func parseID(h natsgo.Header, key string, dst *uuid.UUID) error {
	v := h.Get(key)
	if v == "" {
		return nil
	}
	id, err := uuid.Parse(v)
	if err != nil {
		return fmt.Errorf("queue: header %s: %w", key, err)
	}
	*dst = id
	return nil
}
