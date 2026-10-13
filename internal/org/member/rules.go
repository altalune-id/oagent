package member

import (
	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/authz"
	"altalune.id/oagent/scope"
)

// Requester is who asks for a change before their permissions are known: a person (Operator marks staff) or a non-person operator.
type Requester struct {
	UserID   uuid.UUID
	Operator bool
}

// Actor is who asks for a change, resolved for one org: a person with the permissions they hold there, or a non-person operator.
type Actor struct {
	UserID      uuid.UUID
	Operator    bool
	Permissions []string
}

// Authorize returns nil when a may move target to the role to ("" removes target); it never touches storage.
func Authorize(a Actor, target *Membership, to Role) error {
	if to != "" && !to.IsValid() {
		return &InvalidRoleError{Role: string(to)}
	}
	if a.UserID != uuid.Nil && a.UserID == target.UserID {
		return &SelfChangeError{}
	}
	if a.Operator && a.UserID == uuid.Nil {
		return nil
	}
	if !scope.Grants(a.Permissions, scope.MemberWrite) {
		return &authz.ForbiddenError{Permission: scope.MemberWrite}
	}
	// SECURITY: no one acts on, or grants, more than they hold; this is what keeps admins away from owners.
	if !scope.Covers(a.Permissions, BuiltinPermissions(target.Role)) ||
		(to != "" && !scope.Covers(a.Permissions, BuiltinPermissions(to))) {
		return &RoleNotAllowedError{Role: string(to)}
	}
	return nil
}
