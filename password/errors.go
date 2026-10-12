package password

import (
	"errors"
	"fmt"
	"strconv"

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

// PolicyError reports a password that breaks the policy, with the first violation and the length bounds.
type PolicyError struct {
	Reason   Reason
	Min, Max int
}

func (e *PolicyError) Error() string { return "password: " + e.text() }

// ToAppError maps PolicyError to its own code with the reason and the length bounds.
func (e *PolicyError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeAuthPasswordPolicy, "Password "+e.text()).
		WithMeta(map[string]string{"reason": string(e.Reason), "min": strconv.Itoa(e.Min), "max": strconv.Itoa(e.Max)}).WithCause(e)
}

func (e *PolicyError) text() string {
	switch e.Reason {
	case ReasonTooShort, ReasonTooLong:
		return fmt.Sprintf("must be %d to %d characters", e.Min, e.Max)
	case ReasonCommon:
		return "is too common"
	case ReasonRepeated:
		return "is a repeated pattern"
	case ReasonSequential:
		return "is a simple sequence"
	case ReasonContainsEmail:
		return "must not contain your email"
	case ReasonContainsName:
		return "must not contain your name"
	default:
		return "is not allowed"
	}
}

// IsPolicyError reports whether err's tree contains a *PolicyError.
func IsPolicyError(err error) bool {
	_, ok := errors.AsType[*PolicyError](err)
	return ok
}
