package member

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"altalune.id/oagent/scope"
)

func TestBuiltinPermissions(t *testing.T) {
	assert.Equal(t, []string{"*"}, BuiltinPermissions(RoleOwner))
	assert.False(t, scope.Grants(BuiltinPermissions(RoleAdmin), scope.OrgWrite), "org settings are owner-only")
	assert.True(t, scope.Grants(BuiltinPermissions(RoleAdmin), scope.MemberWrite))
	assert.True(t, scope.Grants(BuiltinPermissions(RoleMember), scope.AgentWrite))
	assert.False(t, scope.Grants(BuiltinPermissions(RoleMember), scope.ProjectWrite))
	assert.False(t, scope.Grants(BuiltinPermissions(RoleMember), scope.MemberWrite))
	assert.Empty(t, BuiltinPermissions("root"))
}

func TestBuiltinPermissions_OnlyCatalogNames(t *testing.T) {
	for _, r := range []Role{RoleOwner, RoleAdmin, RoleMember} {
		for _, p := range BuiltinPermissions(r) {
			if p == "*" {
				continue
			}
			if res, ok := strings.CutSuffix(p, ":*"); ok {
				assert.True(t, scope.Grants([]string{p}, res+":read") || scope.Grants([]string{p}, res+":write"), p)
				assert.True(t, scope.Valid(res+":read") || scope.Valid(res+":write"), "wildcard %q must cover a real resource", p)
				continue
			}
			assert.True(t, scope.Valid(p), p)
		}
	}
}

func TestBuiltinPermissions_AdminMirrorsAgentRights(t *testing.T) {
	admin := BuiltinPermissions(RoleAdmin)
	for _, p := range []string{scope.ProviderWrite, scope.ProviderRead, scope.McpserverWrite, scope.McpserverRead, scope.RunCreate, scope.RunRead, scope.ThreadRead} {
		assert.True(t, scope.Grants(admin, p), "admin %s", p)
	}
}

func TestBuiltinPermissions_MemberBindsButDoesNotCreate(t *testing.T) {
	m := BuiltinPermissions(RoleMember)
	assert.True(t, scope.Grants(m, scope.ProviderRead), "binding a provider to an agent needs provider:read")
	assert.True(t, scope.Grants(m, scope.McpserverRead), "and granting an MCP server's tools needs mcpserver:read")
	assert.False(t, scope.Grants(m, scope.ProviderWrite), "a member never creates a provider")
	assert.False(t, scope.Grants(m, scope.McpserverWrite))
	assert.True(t, scope.Grants(m, scope.ThreadRead))
}

func TestBuiltinPermissions_MemberKeyCanRunAgents(t *testing.T) {
	assert.True(t, scope.Covers(BuiltinPermissions(RoleMember), []string{scope.RunCreate, scope.RunRead}),
		"a member's personal key may hold the data plane's scopes")
}

func TestBuiltinPermissions_NoRoleLosesAccess(t *testing.T) {
	before := map[Role][]string{
		RoleAdmin:  {scope.OrgRead, "project:*", "agent:*", "webhook:*", "member:*", "apikey:*"},
		RoleMember: {scope.OrgRead, scope.ProjectRead, "agent:*", "webhook:*", scope.MemberRead, scope.ApikeyRead},
	}
	for r, old := range before {
		assert.True(t, scope.Covers(BuiltinPermissions(r), old), "%s keeps every right it had", r)
	}
}
