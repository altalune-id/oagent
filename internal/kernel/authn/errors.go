package authn

import (
	"errors"

	"altalune.id/oagent/apperror"
)

// UnauthorizedError is the single opaque authentication failure.
type UnauthorizedError struct{ causes error }

func (*UnauthorizedError) Error() string { return "authn: unauthorized" }

// Causes returns why each authenticator refused, for server-side logging only. SECURITY: never reachable through Error or Unwrap.
func (e *UnauthorizedError) Causes() error {
	if e == nil {
		return nil
	}
	return e.causes
}

// IsUnauthorizedError reports whether err wraps an *UnauthorizedError.
func IsUnauthorizedError(err error) bool {
	_, ok := errors.AsType[*UnauthorizedError](err)
	return ok
}

// ToAppError maps UnauthorizedError to the canonical Unauthenticated envelope.
func (e *UnauthorizedError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeUnauthenticated, "Sign in required").WithCause(e)
}
