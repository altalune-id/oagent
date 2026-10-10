package password

import (
	"errors"
	"fmt"

	"altalune.id/oagent/apperror"
)

// MismatchError reports that a password does not match its hash.
type MismatchError struct{}

func (*MismatchError) Error() string { return "password: mismatch" }

// IsMismatchError reports whether err's tree contains a *MismatchError.
func IsMismatchError(err error) bool {
	_, ok := errors.AsType[*MismatchError](err)
	return ok
}

// InvalidHashError reports a malformed or unsafe PHC hash string.
type InvalidHashError struct{}

func (*InvalidHashError) Error() string { return "password: invalid hash format" }

// IsInvalidHashError reports whether err's tree contains an *InvalidHashError.
func IsInvalidHashError(err error) bool {
	_, ok := errors.AsType[*InvalidHashError](err)
	return ok
}

// PolicyError reports a password outside the allowed length.
type PolicyError struct{ Min, Max int }

func (e *PolicyError) Error() string {
	return fmt.Sprintf("password: must be %d to %d characters", e.Min, e.Max)
}

// ToAppError maps PolicyError to the validation envelope.
func (e *PolicyError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeValidation, fmt.Sprintf("Password must be %d to %d characters", e.Min, e.Max)).WithCause(e)
}

// IsPolicyError reports whether err's tree contains a *PolicyError.
func IsPolicyError(err error) bool {
	_, ok := errors.AsType[*PolicyError](err)
	return ok
}
