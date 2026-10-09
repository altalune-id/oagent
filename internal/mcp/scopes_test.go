package mcp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	agentv1mcp "altalune.id/oagent/gen/go/agent/v1/agentv1mcp"
	orgv1mcp "altalune.id/oagent/gen/go/org/v1/orgv1mcp"
	projectv1mcp "altalune.id/oagent/gen/go/project/v1/projectv1mcp"
	webhookv1mcp "altalune.id/oagent/gen/go/webhook/v1/webhookv1mcp"
	"altalune.id/oagent/scope"
)

// TestScopeTableInCatalog guards the MCP tool→scope table against drift from the shared catalog.
func TestScopeTableInCatalog(t *testing.T) {
	for tool, sc := range ScopeTable() {
		assert.True(t, scope.Valid(sc), tool+" -> "+sc)
	}
}

func TestScopeFor(t *testing.T) {
	assert.Equal(t, scope.OrgWrite, ScopeFor(orgv1mcp.OrgCreateToolName))
	assert.Equal(t, scope.OrgRead, ScopeFor(orgv1mcp.OrgGetToolName))
	assert.Equal(t, scope.OrgRead, ScopeFor(orgv1mcp.OrgListToolName))

	assert.Equal(t, scope.ProjectWrite, ScopeFor(projectv1mcp.ProjectCreateToolName))
	assert.Equal(t, scope.ProjectRead, ScopeFor(projectv1mcp.ProjectGetToolName))
	assert.Equal(t, scope.ProjectRead, ScopeFor(projectv1mcp.ProjectListToolName))

	assert.Equal(t, scope.AgentWrite, ScopeFor(agentv1mcp.AgentCreateToolName))
	assert.Equal(t, scope.AgentRead, ScopeFor(agentv1mcp.AgentGetToolName))
	assert.Equal(t, scope.AgentRead, ScopeFor(agentv1mcp.AgentListToolName))
	assert.Equal(t, scope.AgentWrite, ScopeFor(agentv1mcp.AgentUpdateToolName))
	assert.Equal(t, scope.AgentWrite, ScopeFor(agentv1mcp.AgentDeleteToolName))

	assert.Equal(t, scope.ProjectWrite, ScopeFor(webhookv1mcp.WebhookCreateToolName))
	assert.Equal(t, scope.ProjectRead, ScopeFor(webhookv1mcp.WebhookListToolName))

	assert.Equal(t, "", ScopeFor("unknown"))
}

func TestAllScopes(t *testing.T) {
	assert.Equal(t, []string{scope.AgentRead, scope.AgentWrite, scope.OrgRead, scope.OrgWrite, scope.ProjectRead, scope.ProjectWrite}, AllScopes(context.Background()))
}
