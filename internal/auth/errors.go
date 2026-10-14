package auth

import (
	"errors"
	"strconv"
	"time"

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

// RateLimitedError signals too many attempts; it says nothing about whether the account exists.
type RateLimitedError struct{ RetryAfter time.Duration }

func (*RateLimitedError) Error() string { return "auth: rate limited" }

// IsRateLimitedError reports whether err wraps a *RateLimitedError.
func IsRateLimitedError(err error) bool {
	_, ok := errors.AsType[*RateLimitedError](err)
	return ok
}

// RetryAfterSeconds is RetryAfter rounded up to whole seconds, for a Retry-After header.
func (e *RateLimitedError) RetryAfterSeconds() int {
	return int((e.RetryAfter + time.Second - 1) / time.Second)
}

// Message is the generic text shown to the person, in whole minutes.
func (e *RateLimitedError) Message() string {
	minutes := int((e.RetryAfter + time.Minute - 1) / time.Minute)
	if minutes <= 1 {
		return "Too many attempts. Try again in a minute."
	}
	return "Too many attempts. Try again in " + strconv.Itoa(minutes) + " minutes."
}

// ToAppError maps RateLimitedError to AUT003 with the wait in seconds.
func (e *RateLimitedError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeAuthRateLimited, e.Message()).
		WithMeta(map[string]string{"retry_after": strconv.Itoa(e.RetryAfterSeconds())}).
		WithCause(e)
}
