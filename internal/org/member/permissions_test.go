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
