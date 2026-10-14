package member

import (
	"errors"
	"fmt"

	"github.com/google/uuid"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/authz"
)

// NotFoundError reports that the user is not a member of the org.
type NotFoundError struct{}

func (*NotFoundError) Error() string { return "member: not found" }

// ToAppError maps NotFoundError to the not-found envelope.
func (e *NotFoundError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeMemberNotFound, "Membership not found").WithCause(e)
}

// IsNotFoundError reports whether err's tree contains a *NotFoundError.
func IsNotFoundError(err error) bool {
	_, ok := errors.AsType[*NotFoundError](err)
	return ok
}

// AlreadyMemberError reports a user who already belongs to the org.
type AlreadyMemberError struct{ OrgID, UserID uuid.UUID }

func (*AlreadyMemberError) Error() string { return "member: already a member" }

// ToAppError maps AlreadyMemberError to the conflict envelope.
func (e *AlreadyMemberError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeMemberAlreadyExists, "User is already a member").WithCause(e)
}

// IsAlreadyMemberError reports whether err's tree contains an *AlreadyMemberError.
func IsAlreadyMemberError(err error) bool {
	_, ok := errors.AsType[*AlreadyMemberError](err)
	return ok
}

// InvalidRoleError reports a membership with an unknown role.
type InvalidRoleError struct{ Role string }

func (e *InvalidRoleError) Error() string {
	return fmt.Sprintf("member: unknown role %q", apperror.Echo(e.Role))
}

// ToAppError maps InvalidRoleError to its own code with the role.
func (e *InvalidRoleError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeMemberInvalidRole, fmt.Sprintf("Unknown membership role %q", apperror.Echo(e.Role))).
		WithMeta(map[string]string{"role": apperror.Echo(e.Role)}).WithCause(e)
}

// IsInvalidRoleError reports whether err's tree contains an *InvalidRoleError.
func IsInvalidRoleError(err error) bool {
	_, ok := errors.AsType[*InvalidRoleError](err)
	return ok
}

// LastOwnerReason is the stable cause of a LastOwnerError, sent to clients as meta.reason.
type LastOwnerReason string

// The causes of a LastOwnerError.
const (
	LastOwnerRemove LastOwnerReason = "remove"
	LastOwnerDemote LastOwnerReason = "demote"
	LastOwnerLeave  LastOwnerReason = "leave"
)

// LastOwnerError reports a change that would leave the org without an owner.
type LastOwnerError struct{ Reason LastOwnerReason }

func (e *LastOwnerError) Error() string { return "member: last owner: " + string(e.Reason) }

// ToAppError maps LastOwnerError to its own code with the reason.
func (e *LastOwnerError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeMemberLastOwner, "An organization must keep at least one owner").
		WithMeta(map[string]string{"reason": string(e.Reason)}).WithCause(e)
}

// IsLastOwnerError reports whether err's tree contains a *LastOwnerError.
func IsLastOwnerError(err error) bool {
	_, ok := errors.AsType[*LastOwnerError](err)
	return ok
}

// RoleNotAllowedError reports a non-owner granting, changing or removing the owner role; Role is the target role, empty for a removal.
type RoleNotAllowedError struct{ Role string }

func (e *RoleNotAllowedError) Error() string {
	return fmt.Sprintf("member: role %q not allowed", apperror.Echo(e.Role))
}

// ToAppError maps RoleNotAllowedError to the forbidden envelope with the target role.
func (e *RoleNotAllowedError) ToAppError() *apperror.AppError {
	ae := apperror.New(apperror.CodeMemberRoleNotAllowed, "Only an owner can grant, change or remove the owner role").WithCause(e)
	if e.Role != "" {
		ae = ae.WithMeta(map[string]string{"role": apperror.Echo(e.Role)})
	}
	return ae
}

// IsRoleNotAllowedError reports whether err's tree contains a *RoleNotAllowedError.
func IsRoleNotAllowedError(err error) bool {
	_, ok := errors.AsType[*RoleNotAllowedError](err)
	return ok
}

// SelfChangeError reports a person changing their own role or removing themselves instead of leaving.
type SelfChangeError struct{}

func (*SelfChangeError) Error() string { return "member: self change" }

// ToAppError maps SelfChangeError to its own code.
func (e *SelfChangeError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeMemberSelfChange, "Use Leave to leave the organization").WithCause(e)
}

// IsSelfChangeError reports whether err's tree contains a *SelfChangeError.
func IsSelfChangeError(err error) bool {
	_, ok := errors.AsType[*SelfChangeError](err)
	return ok
}

// IsExpectedError reports whether err is a domain outcome callers handle, not an incident to report.
func IsExpectedError(err error) bool {
	return IsNotFoundError(err) || IsAlreadyMemberError(err) || IsInvalidRoleError(err) ||
		IsLastOwnerError(err) || IsRoleNotAllowedError(err) || IsSelfChangeError(err) || authz.IsForbiddenError(err)
}
