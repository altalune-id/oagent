package enduser

import (
	"errors"
	"strconv"

	"altalune.id/oagent/apperror"
)

// NotFoundError reports that no end user matched the lookup.
type NotFoundError struct{ ID string }

func (e *NotFoundError) Error() string {
	return "enduser: end user not found: id=" + apperror.Echo(e.ID)
}

// IsNotFoundError reports whether err's tree contains a *NotFoundError.
func IsNotFoundError(err error) bool {
	_, ok := errors.AsType[*NotFoundError](err)
	return ok
}

// IdentityReason is the stable cause of an InvalidIdentityError, sent to clients as meta.reason.
type IdentityReason string

// The causes of an InvalidIdentityError.
const (
	IdentityKind  IdentityReason = "kind"
	IdentityValue IdentityReason = "value"
)

func (r IdentityReason) text() string {
	if r == IdentityKind {
		return "kind is unknown"
	}
	return "value must be 1 to " + strconv.Itoa(MaxValueLen) + " characters"
}

// InvalidIdentityError reports that an identity breaks its invariants, or that none was given.
type InvalidIdentityError struct{ Reason IdentityReason }

func (e *InvalidIdentityError) Error() string { return "enduser: identity " + e.Reason.text() }

// IsInvalidIdentityError reports whether err's tree contains an *InvalidIdentityError.
func IsInvalidIdentityError(err error) bool {
	_, ok := errors.AsType[*InvalidIdentityError](err)
	return ok
}

// IsExpectedError reports whether err is a typed domain error a service returns as-is.
func IsExpectedError(err error) bool {
	return IsNotFoundError(err) || IsInvalidIdentityError(err)
}

// ToAppError maps NotFoundError to its own code.
func (e *NotFoundError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeEndUserNotFound, "End user not found").WithCause(e)
}

// ToAppError maps InvalidIdentityError to its own code with the reason and the length limit.
func (e *InvalidIdentityError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeEndUserInvalidIdentity, "End user identity "+e.Reason.text()).
		WithMeta(map[string]string{"reason": string(e.Reason), "max": strconv.Itoa(MaxValueLen)}).
		WithCause(e)
}

// InvalidPageTokenError reports a page token this server did not issue.
type InvalidPageTokenError struct{}

func (e *InvalidPageTokenError) Error() string { return "enduser: invalid page token" }

// ToAppError maps InvalidPageTokenError to a validation error on page_token.
func (e *InvalidPageTokenError) ToAppError() *apperror.AppError {
	return apperror.Validation([]apperror.Violation{{Field: "page_token", Reason: apperror.FieldInvalid}})
}
