package member_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"altalune.id/oagent/internal/kernel/authz"
	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/scope"
)

type outcome int

const (
	allowed outcome = iota
	roleNotAllowed
	selfChange
	forbidden
	invalidRole
)

func outcomeOf(err error) outcome {
	switch {
	case err == nil:
		return allowed
	case member.IsRoleNotAllowedError(err):
		return roleNotAllowed
	case member.IsSelfChangeError(err):
		return selfChange
	case member.IsInvalidRoleError(err):
		return invalidRole
	}
	if fe, ok := errors.AsType[*authz.ForbiddenError](err); ok && fe.Permission == scope.MemberWrite {
		return forbidden
	}
	return -1
}

func TestAuthorize(t *testing.T) {
	roles := []member.Role{member.RoleOwner, member.RoleAdmin, member.RoleMember}
	targets := append(append([]member.Role{}, roles...), "")
	actors := map[string]member.Actor{
		"operator":   {Operator: true},
		"staff":      {UserID: uuid.New(), Permissions: member.BuiltinPermissions(member.RoleAdmin)},
		"owner":      {UserID: uuid.New(), Permissions: member.BuiltinPermissions(member.RoleOwner)},
		"admin":      {UserID: uuid.New(), Permissions: member.BuiltinPermissions(member.RoleAdmin)},
		"member":     {UserID: uuid.New(), Permissions: member.BuiltinPermissions(member.RoleMember)},
		"non-member": {UserID: uuid.New()},
	}
	want := func(actor string, from, to member.Role) outcome {
		switch actor {
		case "operator", "owner":
			return allowed
		case "admin", "staff":
			if from == member.RoleOwner || to == member.RoleOwner {
				return roleNotAllowed
			}
			return allowed
		default:
			return forbidden
		}
	}

	for name, a := range actors {
		for _, from := range roles {
			for _, to := range targets {
				t.Run(fmt.Sprintf("%s/%s->%q", name, from, to), func(t *testing.T) {
					target := &member.Membership{UserID: uuid.New(), Role: from}
					err := member.Authorize(a, target, to)
					assert.Equal(t, want(name, from, to), outcomeOf(err), "got %v", err)
				})
			}
		}
	}

	for _, self := range roles {
		for _, to := range targets {
			for _, operator := range []bool{false, true} {
				t.Run(fmt.Sprintf("self %s->%q operator=%v", self, to, operator), func(t *testing.T) {
					a := member.Actor{UserID: uuid.New(), Operator: operator, Permissions: member.BuiltinPermissions(self)}
					err := member.Authorize(a, &member.Membership{UserID: a.UserID, Role: self}, to)
					assert.Equal(t, selfChange, outcomeOf(err), "got %v", err)
				})
			}
		}
	}

	for name, a := range actors {
		t.Run(name+" invalid role", func(t *testing.T) {
			err := member.Authorize(a, &member.Membership{UserID: uuid.New(), Role: member.RoleMember}, "root")
			assert.Equal(t, invalidRole, outcomeOf(err), "got %v", err)
		})
	}
}

func TestAuthorize_RoleNotAllowedNamesTheTargetRole(t *testing.T) {
	admin := member.Actor{UserID: uuid.New(), Permissions: member.BuiltinPermissions(member.RoleAdmin)}
	err := member.Authorize(admin, &member.Membership{UserID: uuid.New(), Role: member.RoleOwner}, member.RoleAdmin)
	rn, ok := errors.AsType[*member.RoleNotAllowedError](err)
	assert.True(t, ok)
	assert.Equal(t, string(member.RoleAdmin), rn.Role)
}

// SECURITY: a narrow role may manage members only up to what it holds itself.
func TestAuthorize_CustomRoleSet(t *testing.T) {
	lead := member.Actor{UserID: uuid.New(), Permissions: []string{scope.MemberRead, scope.MemberWrite}}
	peer := &member.Membership{UserID: uuid.New()}
	assert.NoError(t, member.Authorize(lead, peer, ""), "a person with no role yet (an add) is within reach")

	plain := &member.Membership{UserID: uuid.New(), Role: member.RoleMember}
	assert.True(t, member.IsRoleNotAllowedError(member.Authorize(lead, plain, member.RoleMember)), "a member holds agent:*, the lead does not")
	assert.True(t, member.IsRoleNotAllowedError(member.Authorize(lead, plain, "")), "the lead cannot remove someone who holds more")
	assert.True(t, member.IsRoleNotAllowedError(member.Authorize(lead, peer, member.RoleMember)), "the lead cannot grant what it lacks")

	noWrite := member.Actor{UserID: uuid.New(), Permissions: []string{"agent:*", scope.MemberRead}}
	assert.True(t, authz.IsForbiddenError(member.Authorize(noWrite, peer, "")))
}

// SECURITY: Operator skips the checks only for a non-person operator; a person flagged as operator stays capped by what they hold.
func TestAuthorize_OperatorPersonIsCapped(t *testing.T) {
	owner := &member.Membership{UserID: uuid.New(), Role: member.RoleOwner}
	admin := member.Actor{UserID: uuid.New(), Operator: true, Permissions: member.BuiltinPermissions(member.RoleAdmin)}
	assert.True(t, member.IsRoleNotAllowedError(member.Authorize(admin, owner, member.RoleMember)))

	bare := member.Actor{UserID: uuid.New(), Operator: true}
	assert.True(t, authz.IsForbiddenError(member.Authorize(bare, owner, member.RoleMember)))
}
