package mcp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	orgv1mcp "altalune.id/oagent/gen/go/org/v1/orgv1mcp"
	projectv1mcp "altalune.id/oagent/gen/go/project/v1/projectv1mcp"
)

func TestScopeFor(t *testing.T) {
	assert.Equal(t, ScopeOrgWrite, ScopeFor(orgv1mcp.OrgCreateToolName))
	assert.Equal(t, ScopeOrgRead, ScopeFor(orgv1mcp.OrgGetToolName))
	assert.Equal(t, ScopeOrgRead, ScopeFor(orgv1mcp.OrgListToolName))

	assert.Equal(t, ScopeProjectWrite, ScopeFor(projectv1mcp.ProjectCreateToolName))
	assert.Equal(t, ScopeProjectRead, ScopeFor(projectv1mcp.ProjectGetToolName))
	assert.Equal(t, ScopeProjectRead, ScopeFor(projectv1mcp.ProjectListToolName))

	assert.Equal(t, "", ScopeFor("unknown"))
}

func TestAllScopes(t *testing.T) {
	assert.Equal(t, []string{ScopeOrgRead, ScopeOrgWrite, ScopeProjectRead, ScopeProjectWrite}, AllScopes(context.Background()))
}
