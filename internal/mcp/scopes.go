// Package mcp wires the MCP surface: tool scopes and error payloads.
package mcp

import (
	"context"
	"maps"
	"slices"

	agentv1mcp "altalune.id/oagent/gen/go/agent/v1/agentv1mcp"
	orgv1mcp "altalune.id/oagent/gen/go/org/v1/orgv1mcp"
	projectv1mcp "altalune.id/oagent/gen/go/project/v1/projectv1mcp"
	webhookv1mcp "altalune.id/oagent/gen/go/webhook/v1/webhookv1mcp"
	"altalune.id/oagent/scope"
)

// ScopeTable maps every published tool to the scope its caller must hold.
func ScopeTable() map[string]string {
	return map[string]string{
		orgv1mcp.OrgCreateToolName: scope.OrgWrite,
		orgv1mcp.OrgGetToolName:    scope.OrgRead,
		orgv1mcp.OrgListToolName:   scope.OrgRead,

		projectv1mcp.ProjectCreateToolName: scope.ProjectWrite,
		projectv1mcp.ProjectGetToolName:    scope.ProjectRead,
		projectv1mcp.ProjectListToolName:   scope.ProjectRead,

		agentv1mcp.AgentCreateToolName: scope.AgentWrite,
		agentv1mcp.AgentGetToolName:    scope.AgentRead,
		agentv1mcp.AgentListToolName:   scope.AgentRead,
		agentv1mcp.AgentUpdateToolName: scope.AgentWrite,
		agentv1mcp.AgentDeleteToolName: scope.AgentWrite,

		webhookv1mcp.WebhookCreateToolName:      scope.ProjectWrite,
		webhookv1mcp.WebhookListToolName:        scope.ProjectRead,
		webhookv1mcp.WebhookDeliveriesToolName:  scope.ProjectRead,
		webhookv1mcp.WebhookRetryFailedToolName: scope.ProjectWrite,
	}
}

// ScopeFor returns the scope tool requires, or "" for an unknown tool (the server denies it).
func ScopeFor(tool string) string { return ScopeTable()[tool] }

// AllScopes is the allow-all ScopesFunc used until auth lands.
func AllScopes(context.Context) []string {
	seen := map[string]struct{}{}
	for _, s := range ScopeTable() {
		seen[s] = struct{}{}
	}
	return slices.Sorted(maps.Keys(seen))
}
