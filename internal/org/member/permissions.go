package member

import "altalune.id/oagent/scope"

// BuiltinPermissions returns the permissions r grants inside its org.
func BuiltinPermissions(r Role) []string {
	switch r {
	case RoleOwner:
		return []string{"*"}
	case RoleAdmin:
		return []string{
			scope.OrgRead, "project:*", "agent:*", "webhook:*", "member:*", "apikey:*", "provider:*", "mcpserver:*", "run:*", scope.ThreadRead,
		}
	case RoleMember:
		return []string{
			scope.OrgRead, scope.ProjectRead, "agent:*", "webhook:*", scope.MemberRead, scope.ApikeyRead,
			scope.ProviderRead, scope.McpserverRead, "run:*", scope.ThreadRead,
		}
	}
	return nil
}
