package mcp

import (
	"context"
	"errors"

	"go.opentelemetry.io/otel/trace"

	"altalune.id/oagent/apperror"
	rootmcp "altalune.id/oagent/mcp"
	"altalune.id/oagent/reqid"
)

// ErrorPayload maps a tool failure onto the caller-visible payload; an empty Code leaves it unmapped.
func ErrorPayload(ctx context.Context, err error) rootmcp.ErrorPayload {
	if denied, ok := errors.AsType[*rootmcp.ScopeDeniedError](err); ok {
		return AttachContext(ctx, rootmcp.ErrorPayload{
			Code:    "FORBIDDEN",
			Message: denied.Error(),
			Meta:    map[string]string{"tool": denied.Tool, "scope": denied.Scope},
		})
	}
	if invalid, ok := errors.AsType[*rootmcp.InvalidArgumentsError](err); ok {
		meta := map[string]string{"tool": invalid.Tool}
		if invalid.Field != "" {
			meta["field"] = invalid.Field
		}
		return AttachContext(ctx, rootmcp.ErrorPayload{Code: "INVALID_ARGUMENT", Message: invalid.Error(), Meta: meta})
	}
	if ae, ok := apperror.AsAppError(err); ok {
		return AttachContext(ctx, rootmcp.ErrorPayload{
			Code: string(ae.Code()), Message: ae.Message(), Meta: ae.Meta(), Violations: violations(ae.Violations()),
		})
	}
	return rootmcp.ErrorPayload{}
}

// AttachContext fills the request and trace ids when absent.
func AttachContext(ctx context.Context, p rootmcp.ErrorPayload) rootmcp.ErrorPayload {
	if p.RequestID == "" {
		p.RequestID = reqid.FromContext(ctx)
	}
	if p.TraceID == "" {
		if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
			p.TraceID = sc.TraceID().String()
		}
	}
	return p
}

func violations(vs []apperror.Violation) []rootmcp.Violation {
	if len(vs) == 0 {
		return nil
	}
	out := make([]rootmcp.Violation, 0, len(vs))
	for _, v := range vs {
		out = append(out, rootmcp.Violation{Field: v.Field, Reason: string(v.Reason), Param: v.Param})
	}
	return out
}
