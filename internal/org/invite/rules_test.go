package invite_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"altalune.id/oagent/internal/kernel/authz"
	"altalune.id/oagent/internal/org/invite"
	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/scope"
)

func person(role member.Role) member.Actor {
	return member.Actor{UserID: uuid.New(), Permissions: member.BuiltinPermissions(role)}
}

func TestAuthorize(t *testing.T) {
	staff := member.Actor{UserID: uuid.New(), Operator: true, Permissions: member.BuiltinPermissions(member.RoleAdmin)}
	cases := []struct {
		name  string
		actor member.Actor
		role  member.Role
		want  func(error) bool
	}{
		{"owner invites owner", person(member.RoleOwner), member.RoleOwner, nil},
		{"owner invites admin", person(member.RoleOwner), member.RoleAdmin, nil},
		{"owner invites member", person(member.RoleOwner), member.RoleMember, nil},
		{"operator invites owner", member.Actor{Operator: true}, member.RoleOwner, nil},
		{"admin invites admin", person(member.RoleAdmin), member.RoleAdmin, nil},
		{"admin invites member", person(member.RoleAdmin), member.RoleMember, nil},
		{"admin invites owner", person(member.RoleAdmin), member.RoleOwner, member.IsRoleNotAllowedError},
		{"staff invites admin", staff, member.RoleAdmin, nil},
		{"staff invites owner", staff, member.RoleOwner, member.IsRoleNotAllowedError},
		{"member invites member", person(member.RoleMember), member.RoleMember, authz.IsForbiddenError},
		{"non-member invites member", member.Actor{UserID: uuid.New()}, member.RoleMember, authz.IsForbiddenError},
		{"unknown role", person(member.RoleOwner), member.Role("root"), member.IsInvalidRoleError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := invite.Authorize(tc.actor, tc.role)
			if tc.want == nil {
				assert.NoError(t, err)
				return
			}
			assert.True(t, tc.want(err), "got %v", err)
		})
	}
}

func TestAuthorize_CustomRoleSet(t *testing.T) {
	a := member.Actor{UserID: uuid.New(), Permissions: []string{scope.MemberRead, scope.MemberWrite}}
	assert.True(t, member.IsRoleNotAllowedError(invite.Authorize(a, member.RoleMember)),
		"a member holds agent:*, which this set does not")
}
