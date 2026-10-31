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

// UnavailableError signals that a credential could not be checked, such as an identity provider or database outage.
type UnavailableError struct{ Cause error }

func (*UnavailableError) Error() string { return "authn: credential check unavailable" }

func (e *UnavailableError) Unwrap() error { return e.Cause }

// ToAppError maps UnavailableError to the TokenUnavailable envelope. SECURITY: the cause is logged, never sent.
func (e *UnavailableError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeTokenUnavailable, "Credential verification unavailable, try again").WithCause(e)
}

// IsUnavailableError reports whether err wraps an *UnavailableError.
func IsUnavailableError(err error) bool {
	_, ok := errors.AsType[*UnavailableError](err)
	return ok
}
