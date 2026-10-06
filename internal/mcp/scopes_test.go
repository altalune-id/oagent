package mcp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	orgv1mcp "altalune.id/auth-server/gen/go/org/v1/orgv1mcp"
)

func TestScopeFor(t *testing.T) {
	assert.Equal(t, ScopeOrgWrite, ScopeFor(orgv1mcp.OrgCreateToolName))
	assert.Equal(t, ScopeOrgRead, ScopeFor(orgv1mcp.OrgGetToolName))
	assert.Equal(t, "", ScopeFor("unknown"))
}

func TestAllScopes(t *testing.T) {
	assert.Equal(t, []string{ScopeOrgRead, ScopeOrgWrite}, AllScopes(context.Background()))
}
