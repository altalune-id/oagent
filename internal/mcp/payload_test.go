package mcp

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"altalune.id/auth-server/apperror"
	rootmcp "altalune.id/auth-server/mcp"
	"altalune.id/auth-server/reqid"
)

func TestErrorPayload_App(t *testing.T) {
	p := ErrorPayload(context.Background(), apperror.New(apperror.CodeOrgNotFound, "Organization not found"))
	assert.Equal(t, "ORG001", p.Code)
	assert.Equal(t, "Organization not found", p.Message)
}

func TestErrorPayload_ScopeDenied(t *testing.T) {
	p := ErrorPayload(context.Background(), &rootmcp.ScopeDeniedError{Tool: "org_get", Scope: ScopeOrgRead})
	assert.Equal(t, "FORBIDDEN", p.Code)
	assert.Equal(t, "org_get", p.Meta["tool"])
}

func TestErrorPayload_InvalidArguments(t *testing.T) {
	p := ErrorPayload(context.Background(), &rootmcp.InvalidArgumentsError{Tool: "org_get", Field: "id", Reason: "bad"})
	assert.Equal(t, "INVALID_ARGUMENT", p.Code)
	assert.Equal(t, "id", p.Meta["field"])
}

func TestErrorPayload_SkipsEmptyField(t *testing.T) {
	p := ErrorPayload(context.Background(), &rootmcp.InvalidArgumentsError{Tool: "org_get", Reason: "bad"})
	assert.NotContains(t, p.Meta, "field")
}

func TestErrorPayload_Unmapped(t *testing.T) {
	assert.Empty(t, ErrorPayload(context.Background(), errors.New("boom")).Code)
}

func TestAttachContext(t *testing.T) {
	ctx := reqid.WithContext(context.Background(), "r1")
	assert.Equal(t, "r1", AttachContext(ctx, rootmcp.ErrorPayload{}).RequestID)

	kept := AttachContext(ctx, rootmcp.ErrorPayload{RequestID: "x", TraceID: "y"})
	assert.Equal(t, "x", kept.RequestID)
	assert.Equal(t, "y", kept.TraceID)
}
