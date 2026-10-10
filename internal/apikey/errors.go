package apikey

import (
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"

	"altalune.id/oagent/apperror"
)

// NotFoundError signals a missing api key row.
type NotFoundError struct {
	ID string
}

func (e *NotFoundError) Error() string {
	if e == nil || e.ID == "" {
		return "apikey: not found"
	}
	return fmt.Sprintf("apikey: not found: id=%s", e.ID)
}

// IsNotFoundError reports whether err wraps a *NotFoundError.
func IsNotFoundError(err error) bool {
	_, ok := errors.AsType[*NotFoundError](err)
	return ok
}

// InvalidNameError signals a name that fails the aggregate invariants.
type InvalidNameError struct{ Reason string }

func (e *InvalidNameError) Error() string {
	if e.Reason == "" {
		return "apikey: invalid name"
	}
	return "apikey: invalid name: " + e.Reason
}

// UnknownScopeError signals a scope outside the catalog.
type UnknownScopeError struct{ Scope string }

func (e *UnknownScopeError) Error() string {
	return fmt.Sprintf("apikey: unknown scope: %q", e.Scope)
}

// ScopeLevelError signals a project key granted a scope above its level.
type ScopeLevelError struct{ Scope string }

func (e *ScopeLevelError) Error() string {
	return fmt.Sprintf("apikey: scope %q is not project-level", e.Scope)
}

// PersonOnlyScopeError signals a person-only scope requested for a key.
type PersonOnlyScopeError struct{ Scope string }

func (e *PersonOnlyScopeError) Error() string {
	return fmt.Sprintf("apikey: scope %q can only be held by a person", e.Scope)
}

// ExpiryRequiredError signals a missing expiry at mint.
type ExpiryRequiredError struct{}

func (e *ExpiryRequiredError) Error() string { return "apikey: expires_at is required" }

// ExpiryInPastError signals an expiry not strictly in the future.
type ExpiryInPastError struct{}

func (e *ExpiryInPastError) Error() string { return "apikey: expires_at must be in the future" }

// ExpiryTooLongError signals an expiry beyond MaxLifetime.
type ExpiryTooLongError struct{}

func (e *ExpiryTooLongError) Error() string {
	return "apikey: expires_at is more than one year away"
}

// GrantConflictError signals both All and named project ids set.
type GrantConflictError struct{}

func (e *GrantConflictError) Error() string {
	return "apikey: grant cannot set all_projects and project_ids"
}

// EmptyGrantError signals a grant naming nothing.
type EmptyGrantError struct{}

func (e *EmptyGrantError) Error() string {
	return "apikey: org key grant must set all_projects or project_ids"
}

// IsInvalidNameError reports whether err wraps an *InvalidNameError.
func IsInvalidNameError(err error) bool {
	_, ok := errors.AsType[*InvalidNameError](err)
	return ok
}

// IsUnknownScopeError reports whether err wraps an *UnknownScopeError.
func IsUnknownScopeError(err error) bool {
	_, ok := errors.AsType[*UnknownScopeError](err)
	return ok
}

// IsScopeLevelError reports whether err wraps a *ScopeLevelError.
func IsScopeLevelError(err error) bool {
	_, ok := errors.AsType[*ScopeLevelError](err)
	return ok
}

// IsPersonOnlyScopeError reports whether err wraps a *PersonOnlyScopeError.
func IsPersonOnlyScopeError(err error) bool {
	_, ok := errors.AsType[*PersonOnlyScopeError](err)
	return ok
}

// IsExpiryError reports whether err wraps one of the expiry errors.
func IsExpiryError(err error) bool {
	if _, ok := errors.AsType[*ExpiryRequiredError](err); ok {
		return true
	}
	if _, ok := errors.AsType[*ExpiryInPastError](err); ok {
		return true
	}
	_, ok := errors.AsType[*ExpiryTooLongError](err)
	return ok
}

// IsGrantError reports whether err wraps one of the grant errors.
func IsGrantError(err error) bool {
	if _, ok := errors.AsType[*GrantConflictError](err); ok {
		return true
	}
	_, ok := errors.AsType[*EmptyGrantError](err)
	return ok
}

// IsExpectedError reports whether err is a typed domain error a service returns as-is.
func IsExpectedError(err error) bool {
	return IsNotFoundError(err) || IsInvalidNameError(err) || IsUnknownScopeError(err) ||
		IsScopeLevelError(err) || IsPersonOnlyScopeError(err) || IsExpiryError(err) || IsGrantError(err)
}

// ToAppError maps NotFoundError to the canonical NotFound envelope.
func (e *NotFoundError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeAPIKeyNotFound, "API key not found").WithCause(e)
}

// ToAppError maps InvalidNameError to the canonical Validation envelope.
func (e *InvalidNameError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeAPIKeyInvalidName, "Invalid API key name").WithCause(e)
}

// ToAppError maps UnknownScopeError to the canonical Validation envelope.
func (e *UnknownScopeError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeAPIKeyInvalidScope, "Unknown API key scope").WithCause(e)
}

// ToAppError maps ScopeLevelError to the canonical Validation envelope.
func (e *ScopeLevelError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeAPIKeyInvalidScope, "Scope level too high for a project key").WithCause(e)
}

// ToAppError maps PersonOnlyScopeError to the canonical Validation envelope.
func (e *PersonOnlyScopeError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeAPIKeyPersonOnlyScope, fmt.Sprintf("Scope %s can only be held by a person", e.Scope)).WithCause(e)
}

// ToAppError maps expiry errors to the canonical Validation envelope.
func (e *ExpiryRequiredError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeAPIKeyInvalidExpiry, "expires_at is required").WithCause(e)
}

// ToAppError maps ExpiryInPastError to the canonical Validation envelope.
func (e *ExpiryInPastError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeAPIKeyInvalidExpiry, "expires_at must be in the future").WithCause(e)
}

// ToAppError maps ExpiryTooLongError to the canonical Validation envelope.
func (e *ExpiryTooLongError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeAPIKeyInvalidExpiry, "expires_at is more than one year away").WithCause(e)
}

// ToAppError maps grant errors to the canonical Validation envelope.
func (e *GrantConflictError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeAPIKeyInvalidGrant, "Grant cannot set all_projects and project_ids").WithCause(e)
}

// ToAppError maps EmptyGrantError to the canonical Validation envelope.
func (e *EmptyGrantError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeAPIKeyInvalidGrant, "Org key grant cannot be empty").WithCause(e)
}

// IsUniqueViolation reports whether err is a Postgres unique-violation (SQLSTATE 23505).
func IsUniqueViolation(err error) bool {
	pgErr, ok := errors.AsType[*pgconn.PgError](err)
	return ok && pgErr.Code == "23505"
}
