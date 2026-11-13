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
			"channel:*", scope.ApprovalDecide, "knowledge:*", "memory:*",
		}
	case RoleMember:
		return []string{
			scope.OrgRead, scope.ProjectRead, "agent:*", "webhook:*", scope.MemberRead, scope.ApikeyRead,
			scope.ProviderRead, scope.McpserverRead, "run:*", scope.ThreadRead, scope.ChannelRead, scope.ApprovalDecide,
			scope.KnowledgeRead, scope.MemoryRead,
		}
	}
	return nil
}
