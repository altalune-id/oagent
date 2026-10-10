package auth

import (
	"errors"

	"altalune.id/oagent/apperror"
)

// InvalidCredentialsError signals a failed sign-in. SECURITY: one error for every cause, so emails cannot be enumerated.
type InvalidCredentialsError struct{}

func (*InvalidCredentialsError) Error() string { return "auth: invalid credentials" }

// IsInvalidCredentialsError reports whether err wraps an *InvalidCredentialsError.
func IsInvalidCredentialsError(err error) bool {
	_, ok := errors.AsType[*InvalidCredentialsError](err)
	return ok
}

// ToAppError maps InvalidCredentialsError to the canonical Unauthenticated envelope.
func (e *InvalidCredentialsError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeAuthInvalidCredentials, "Invalid email or password").WithCause(e)
}
