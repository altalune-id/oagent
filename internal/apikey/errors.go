package apikey

import (
	"errors"
	"fmt"
	"strconv"
	"time"

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

// HintTakenError signals that a live key in the org already ends with the new key's hint; the service mints again.
type HintTakenError struct{}

func (e *HintTakenError) Error() string { return "apikey: hint already used by a live key in the org" }

// IsHintTakenError reports whether err wraps a *HintTakenError.
func IsHintTakenError(err error) bool {
	_, ok := errors.AsType[*HintTakenError](err)
	return ok
}

// NameReason is the stable cause of an InvalidNameError, sent to clients as meta.reason.
type NameReason string

// The causes of an InvalidNameError.
const (
	NameRequired NameReason = "required"
	NameLength   NameReason = "length"
)

// InvalidNameError signals a name that fails the aggregate invariants.
type InvalidNameError struct{ Reason NameReason }

func (e *InvalidNameError) Error() string { return "apikey: invalid name: " + e.text() }

func (e *InvalidNameError) text() string {
	if e.Reason == NameRequired {
		return "name is required"
	}
	return fmt.Sprintf("name must be at most %d characters", nameMaxLen)
}

// UnknownScopeError signals a scope outside the catalog.
type UnknownScopeError struct{ Scope string }

func (e *UnknownScopeError) Error() string {
	return fmt.Sprintf("apikey: unknown scope: %q", apperror.Echo(e.Scope))
}

// ScopeLevelError signals a project key granted a scope above its level.
type ScopeLevelError struct{ Scope string }

func (e *ScopeLevelError) Error() string {
	return fmt.Sprintf("apikey: scope %q is not project-level", apperror.Echo(e.Scope))
}

// PersonOnlyScopeError signals a person-only scope requested for a key.
type PersonOnlyScopeError struct{ Scope string }

func (e *PersonOnlyScopeError) Error() string {
	return fmt.Sprintf("apikey: scope %q can only be held by a person", apperror.Echo(e.Scope))
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
	return fmt.Sprintf("apikey: expires_at is more than %d days away", maxLifetimeDays)
}

const maxLifetimeDays = int(MaxLifetime / (24 * time.Hour))

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

// GrantOutsideOrgError signals a grant naming a project that is not in the key's org.
type GrantOutsideOrgError struct{}

func (e *GrantOutsideOrgError) Error() string {
	return "apikey: grant names a project outside the organization"
}

// ScopeExceedsRoleError signals a personal key asking for a scope its owner's role does not grant.
type ScopeExceedsRoleError struct{ Scope string }

func (e *ScopeExceedsRoleError) Error() string {
	return fmt.Sprintf("apikey: scope %q exceeds the owner's role", apperror.Echo(e.Scope))
}

// PersonalKeysDisabledError signals a personal key requested where the deployment has no people to own one.
type PersonalKeysDisabledError struct{}

func (*PersonalKeysDisabledError) Error() string { return "apikey: personal keys are disabled" }

// IsScopeExceedsRoleError reports whether err wraps a *ScopeExceedsRoleError.
func IsScopeExceedsRoleError(err error) bool {
	_, ok := errors.AsType[*ScopeExceedsRoleError](err)
	return ok
}

// IsPersonalKeysDisabledError reports whether err wraps a *PersonalKeysDisabledError.
func IsPersonalKeysDisabledError(err error) bool {
	_, ok := errors.AsType[*PersonalKeysDisabledError](err)
	return ok
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
	if _, ok := errors.AsType[*EmptyGrantError](err); ok {
		return true
	}
	_, ok := errors.AsType[*GrantOutsideOrgError](err)
	return ok
}

// IsExpectedError reports whether err is a typed domain error a service returns as-is.
func IsExpectedError(err error) bool {
	return IsNotFoundError(err) || IsInvalidNameError(err) || IsUnknownScopeError(err) ||
		IsScopeLevelError(err) || IsPersonOnlyScopeError(err) || IsExpiryError(err) || IsGrantError(err) ||
		IsScopeExceedsRoleError(err) || IsPersonalKeysDisabledError(err)
}

// ToAppError maps NotFoundError to the canonical NotFound envelope.
func (e *NotFoundError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeAPIKeyNotFound, "API key not found").WithCause(e)
}

// ToAppError maps InvalidNameError to its own code with the reason and the length limit.
func (e *InvalidNameError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeAPIKeyInvalidName, "API key "+e.text()).
		WithMeta(map[string]string{"reason": string(e.Reason), "max": strconv.Itoa(nameMaxLen)}).WithCause(e)
}

// ToAppError maps UnknownScopeError to the invalid-scope code with the scope.
func (e *UnknownScopeError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeAPIKeyInvalidScope, "Unknown scope "+apperror.Echo(e.Scope)).
		WithMeta(map[string]string{"scope": apperror.Echo(e.Scope), "reason": "unknown"}).WithCause(e)
}

// ToAppError maps ScopeLevelError to the invalid-scope code with the scope.
func (e *ScopeLevelError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeAPIKeyInvalidScope, fmt.Sprintf("Scope %s cannot be granted to a project key", apperror.Echo(e.Scope))).
		WithMeta(map[string]string{"scope": apperror.Echo(e.Scope), "reason": "level"}).WithCause(e)
}

// ToAppError maps PersonOnlyScopeError to its own code with the scope.
func (e *PersonOnlyScopeError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeAPIKeyPersonOnlyScope, fmt.Sprintf("Scope %s can only be held by a person", apperror.Echo(e.Scope))).
		WithMeta(map[string]string{"scope": apperror.Echo(e.Scope)}).WithCause(e)
}

// ToAppError maps ExpiryRequiredError to the invalid-expiry code.
func (e *ExpiryRequiredError) ToAppError() *apperror.AppError {
	return expiryAppError("expires_at is required", "required").WithCause(e)
}

// ToAppError maps ExpiryInPastError to the invalid-expiry code.
func (e *ExpiryInPastError) ToAppError() *apperror.AppError {
	return expiryAppError("expires_at must be in the future", "past").WithCause(e)
}

// ToAppError maps ExpiryTooLongError to the invalid-expiry code.
func (e *ExpiryTooLongError) ToAppError() *apperror.AppError {
	return expiryAppError(fmt.Sprintf("expires_at is more than %d days away", maxLifetimeDays), "too_far").WithCause(e)
}

func expiryAppError(msg, reason string) *apperror.AppError {
	return apperror.New(apperror.CodeAPIKeyInvalidExpiry, msg).
		WithMeta(map[string]string{"reason": reason, "max_days": strconv.Itoa(maxLifetimeDays)})
}

// ToAppError maps GrantConflictError to the invalid-grant code.
func (e *GrantConflictError) ToAppError() *apperror.AppError {
	return grantAppError("Grant cannot set all_projects and project_ids", "conflict").WithCause(e)
}

// ToAppError maps EmptyGrantError to the invalid-grant code.
func (e *EmptyGrantError) ToAppError() *apperror.AppError {
	return grantAppError("Org key grant cannot be empty", "empty").WithCause(e)
}

// ToAppError maps GrantOutsideOrgError to the invalid-grant code.
func (e *GrantOutsideOrgError) ToAppError() *apperror.AppError {
	return grantAppError("Project is not in this organization", "outside_org").WithCause(e)
}

func grantAppError(msg, reason string) *apperror.AppError {
	return apperror.New(apperror.CodeAPIKeyInvalidGrant, msg).WithMeta(map[string]string{"reason": reason})
}

// ToAppError maps ScopeExceedsRoleError to its own code: with the scope, or reason not_member when no role was found.
func (e *ScopeExceedsRoleError) ToAppError() *apperror.AppError {
	if e.Scope == "" {
		return apperror.New(apperror.CodeAPIKeyScopeExceedsRole, "A personal key cannot hold more than your role allows").
			WithMeta(map[string]string{"reason": "not_member"}).WithCause(e)
	}
	return apperror.New(apperror.CodeAPIKeyScopeExceedsRole, fmt.Sprintf("Your role does not allow scope %s", apperror.Echo(e.Scope))).
		WithMeta(map[string]string{"scope": apperror.Echo(e.Scope)}).WithCause(e)
}

// ToAppError maps PersonalKeysDisabledError to the canonical FailedPrecondition envelope.
func (e *PersonalKeysDisabledError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeAPIKeyPersonalKeysDisabled, "Personal API keys are not available on this instance").WithCause(e)
}
