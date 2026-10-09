// Package mcp wires the MCP surface: tool scopes and error payloads.
package mcp

import (
	"context"
	"maps"
	"slices"

	agentv1mcp "altalune.id/oagent/gen/go/agent/v1/agentv1mcp"
	orgv1mcp "altalune.id/oagent/gen/go/org/v1/orgv1mcp"
	projectv1mcp "altalune.id/oagent/gen/go/project/v1/projectv1mcp"
)

const (
	ScopeOrgRead      = "org:read"
	ScopeOrgWrite     = "org:write"
	ScopeProjectRead  = "project:read"
	ScopeProjectWrite = "project:write"
	ScopeAgentRead    = "agent:read"
	ScopeAgentWrite   = "agent:write"
)

// ScopeTable maps every published tool to the scope its caller must hold.
func ScopeTable() map[string]string {
	return map[string]string{
		orgv1mcp.OrgCreateToolName: ScopeOrgWrite,
		orgv1mcp.OrgGetToolName:    ScopeOrgRead,
		orgv1mcp.OrgListToolName:   ScopeOrgRead,

		projectv1mcp.ProjectCreateToolName: ScopeProjectWrite,
		projectv1mcp.ProjectGetToolName:    ScopeProjectRead,
		projectv1mcp.ProjectListToolName:   ScopeProjectRead,

		agentv1mcp.AgentCreateToolName: ScopeAgentWrite,
		agentv1mcp.AgentGetToolName:    ScopeAgentRead,
		agentv1mcp.AgentListToolName:   ScopeAgentRead,
		agentv1mcp.AgentUpdateToolName: ScopeAgentWrite,
		agentv1mcp.AgentDeleteToolName: ScopeAgentWrite,
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
