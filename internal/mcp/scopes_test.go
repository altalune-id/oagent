package mcp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	agentv1mcp "altalune.id/oagent/gen/go/agent/v1/agentv1mcp"
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

	assert.Equal(t, ScopeAgentWrite, ScopeFor(agentv1mcp.AgentCreateToolName))
	assert.Equal(t, ScopeAgentRead, ScopeFor(agentv1mcp.AgentGetToolName))
	assert.Equal(t, ScopeAgentRead, ScopeFor(agentv1mcp.AgentListToolName))
	assert.Equal(t, ScopeAgentWrite, ScopeFor(agentv1mcp.AgentUpdateToolName))
	assert.Equal(t, ScopeAgentWrite, ScopeFor(agentv1mcp.AgentDeleteToolName))

	assert.Equal(t, "", ScopeFor("unknown"))
}

func TestAllScopes(t *testing.T) {
	assert.Equal(t, []string{ScopeAgentRead, ScopeAgentWrite, ScopeOrgRead, ScopeOrgWrite, ScopeProjectRead, ScopeProjectWrite}, AllScopes(context.Background()))
}
