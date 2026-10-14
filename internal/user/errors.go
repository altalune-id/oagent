package user

import (
	"errors"
	"fmt"
	"strconv"

	"altalune.id/oagent/apperror"
)

// NotFoundError reports a missing user.
type NotFoundError struct{}

func (*NotFoundError) Error() string { return "user: not found" }

// ToAppError maps NotFoundError to the not-found envelope.
func (e *NotFoundError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeUserNotFound, "User not found").WithCause(e)
}

// IsNotFoundError reports whether err's tree contains a *NotFoundError.
func IsNotFoundError(err error) bool {
	_, ok := errors.AsType[*NotFoundError](err)
	return ok
}

// UnknownEmailError reports that no user has the email an operator asked for; it is a NotFoundError.
type UnknownEmailError struct{ Email string }

// SECURITY: Error leaves the address out because errors reach logs; only the operator-facing envelope echoes it.
func (*UnknownEmailError) Error() string { return "user: no user with that email" }

func (*UnknownEmailError) Unwrap() error { return &NotFoundError{} }

// ToAppError maps UnknownEmailError to the not-found envelope, naming the email.
func (e *UnknownEmailError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeUserNotFound, "No user with email "+apperror.Echo(e.Email)).WithCause(e)
}

// IsUnknownEmailError reports whether err's tree contains an *UnknownEmailError.
func IsUnknownEmailError(err error) bool {
	_, ok := errors.AsType[*UnknownEmailError](err)
	return ok
}

// AlreadyExistsError reports an email already taken by another user.
type AlreadyExistsError struct{ Email string }

func (*AlreadyExistsError) Error() string { return "user: email already in use" }

// ToAppError maps AlreadyExistsError to the conflict envelope.
func (e *AlreadyExistsError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeUserAlreadyExists, "Email already in use").WithCause(e)
}

// IsAlreadyExistsError reports whether err's tree contains an *AlreadyExistsError.
func IsAlreadyExistsError(err error) bool {
	_, ok := errors.AsType[*AlreadyExistsError](err)
	return ok
}

// InvalidEmailError reports an email that is not a single bare address.
type InvalidEmailError struct{}

func (*InvalidEmailError) Error() string { return "user: invalid email" }

// ToAppError maps InvalidEmailError to the validation envelope.
func (e *InvalidEmailError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeUserInvalidEmail, "Invalid email address").WithCause(e)
}

// IsInvalidEmailError reports whether err's tree contains an *InvalidEmailError.
func IsInvalidEmailError(err error) bool {
	_, ok := errors.AsType[*InvalidEmailError](err)
	return ok
}

// InvalidNameError reports a display name that is too long.
type InvalidNameError struct{}

func (*InvalidNameError) Error() string { return "user: invalid name" }

// ToAppError maps InvalidNameError to the validation envelope.
func (e *InvalidNameError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeUserInvalidName, fmt.Sprintf("Name must be at most %d characters", nameMaxLen)).
		WithMeta(map[string]string{"max": strconv.Itoa(nameMaxLen)}).WithCause(e)
}

// IsInvalidNameError reports whether err's tree contains an *InvalidNameError.
func IsInvalidNameError(err error) bool {
	_, ok := errors.AsType[*InvalidNameError](err)
	return ok
}

// IsExpectedError reports whether err is a domain outcome callers handle, not an incident to report.
func IsExpectedError(err error) bool {
	return IsNotFoundError(err) || IsAlreadyExistsError(err) || IsInvalidEmailError(err) || IsInvalidNameError(err)
}
