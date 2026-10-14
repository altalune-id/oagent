package member

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/authz"
	"altalune.id/oagent/scope"
)

func TestErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		err interface {
			error
			ToAppError() *apperror.AppError
		}
		is   func(error) bool
		code apperror.Code
	}{
		"not found":      {&NotFoundError{}, IsNotFoundError, apperror.CodeMemberNotFound},
		"already member": {&AlreadyMemberError{OrgID: uuid.New(), UserID: uuid.New()}, IsAlreadyMemberError, apperror.CodeMemberAlreadyExists},
		"invalid role":   {&InvalidRoleError{Role: "root"}, IsInvalidRoleError, apperror.CodeMemberInvalidRole},
		"last owner":     {&LastOwnerError{Reason: LastOwnerRemove}, IsLastOwnerError, apperror.CodeMemberLastOwner},
		"role not ok":    {&RoleNotAllowedError{Role: "owner"}, IsRoleNotAllowedError, apperror.CodeMemberRoleNotAllowed},
		"self change":    {&SelfChangeError{}, IsSelfChangeError, apperror.CodeMemberSelfChange},
	} {
		t.Run(name, func(t *testing.T) {
			assert.True(t, tc.is(tc.err))
			assert.True(t, IsExpectedError(tc.err))
			assert.NotEmpty(t, tc.err.Error())
			assert.Equal(t, tc.code, tc.err.ToAppError().Code())
		})
	}
	assert.False(t, IsExpectedError(errors.New("boom")))
	assert.True(t, IsExpectedError(&authz.ForbiddenError{Permission: scope.MemberWrite}))
}

func TestLastOwnerError_SendsItsReason(t *testing.T) {
	for _, r := range []LastOwnerReason{LastOwnerRemove, LastOwnerDemote, LastOwnerLeave} {
		ae := (&LastOwnerError{Reason: r}).ToAppError()
		assert.Equal(t, string(r), ae.Meta()["reason"])
		assert.Equal(t, "An organization must keep at least one owner", ae.Message())
	}
}

func TestRoleNotAllowedError_EchoesTheRole(t *testing.T) {
	ae := (&RoleNotAllowedError{Role: "owner"}).ToAppError()
	assert.Equal(t, "owner", ae.Meta()["role"])
	assert.Equal(t, "Only an owner can grant, change or remove the owner role", ae.Message())
	assert.NotContains(t, (&RoleNotAllowedError{}).ToAppError().Meta(), "role", "removal has no target role")
}

func TestSelfChangeError_Message(t *testing.T) {
	assert.Equal(t, "Use Leave to leave the organization", (&SelfChangeError{}).ToAppError().Message())
}
