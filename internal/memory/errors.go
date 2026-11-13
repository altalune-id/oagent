package memory

import (
	"errors"
	"strconv"

	"altalune.id/oagent/apperror"
)

// NotFoundError reports a fact that is not in the scope asked for, or does not exist.
type NotFoundError struct{ ID string }

func (e *NotFoundError) Error() string { return "memory: fact not found" }

// IsNotFoundError reports whether err's tree contains a *NotFoundError.
func IsNotFoundError(err error) bool {
	_, ok := errors.AsType[*NotFoundError](err)
	return ok
}

// IsExpectedError reports whether err is a typed domain error a service returns as-is.
func IsExpectedError(err error) bool {
	return IsNotFoundError(err) || IsStoreNotFoundError(err) || IsInvalidStoreError(err) || IsStoreInUseError(err) ||
		IsStoreAlreadyExistsError(err) || IsStoreBackendOffError(err)
}

// ToAppError maps NotFoundError to its own code.
func (e *NotFoundError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeMemoryNotFound, "Memory not found").WithCause(e)
}

// IncompleteScopeError reports a scope missing a field the call needs; it is a caller's bug, never an answer.
type IncompleteScopeError struct{ Field string }

func (e *IncompleteScopeError) Error() string { return "memory: incomplete scope: no " + e.Field }

// IsIncompleteScopeError reports whether err's tree contains an *IncompleteScopeError.
func IsIncompleteScopeError(err error) bool {
	_, ok := errors.AsType[*IncompleteScopeError](err)
	return ok
}

// TenantMismatchError reports a scope whose org is not the tenant on ctx; it is refused before any I/O.
type TenantMismatchError struct{}

func (*TenantMismatchError) Error() string { return "memory: scope org is not the tenant on ctx" }

// IsTenantMismatchError reports whether err's tree contains a *TenantMismatchError.
func IsTenantMismatchError(err error) bool {
	_, ok := errors.AsType[*TenantMismatchError](err)
	return ok
}

// StoreNotFoundError reports a memory store that does not exist or that the caller does not reach.
type StoreNotFoundError struct{}

func (*StoreNotFoundError) Error() string { return "memory: store not found" }

// IsStoreNotFoundError reports whether err's tree contains a *StoreNotFoundError.
func IsStoreNotFoundError(err error) bool {
	_, ok := errors.AsType[*StoreNotFoundError](err)
	return ok
}

// ToAppError maps StoreNotFoundError to its own code.
func (e *StoreNotFoundError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeMemoryStoreNotFound, "Memory store not found").WithCause(e)
}

// StoreBackendOffError reports a store whose backend's driver this server does not run, so its facts cannot be reached.
type StoreBackendOffError struct{ Backend Backend }

func (e *StoreBackendOffError) Error() string {
	return "memory: store backend is off: " + string(e.Backend)
}

// IsStoreBackendOffError reports whether err's tree contains a *StoreBackendOffError.
func IsStoreBackendOffError(err error) bool {
	_, ok := errors.AsType[*StoreBackendOffError](err)
	return ok
}

// ToAppError maps StoreBackendOffError to its own code.
func (e *StoreBackendOffError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeMemoryStoreBackendOff, "Memory store's backend is off on this server").WithCause(e)
}

// StoreReason is the stable cause of an InvalidStoreError, sent to clients as meta.reason.
type StoreReason string

// The causes of an InvalidStoreError.
const (
	StoreNameRequired    StoreReason = "required"
	StoreNameTooLong     StoreReason = "too_long"
	StoreBackendDisabled StoreReason = "backend_disabled"
)

// InvalidStoreError reports a store whose name or backend breaks its rules.
type InvalidStoreError struct{ Reason StoreReason }

func (e *InvalidStoreError) Error() string { return "memory: invalid store: " + string(e.Reason) }

// IsInvalidStoreError reports whether err's tree contains an *InvalidStoreError.
func IsInvalidStoreError(err error) bool {
	_, ok := errors.AsType[*InvalidStoreError](err)
	return ok
}

// ToAppError maps InvalidStoreError to its own code with the reason and the name length limit.
func (e *InvalidStoreError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeMemoryStoreInvalid, "Invalid memory store").
		WithMeta(map[string]string{"reason": string(e.Reason), "max": strconv.Itoa(MaxStoreNameLen)}).WithCause(e)
}

// StoreInUseError reports a store that a published agent version uses.
type StoreInUseError struct{}

func (*StoreInUseError) Error() string { return "memory: store is used by a published agent version" }

// IsStoreInUseError reports whether err's tree contains a *StoreInUseError.
func IsStoreInUseError(err error) bool {
	_, ok := errors.AsType[*StoreInUseError](err)
	return ok
}

// ToAppError maps StoreInUseError to its own code.
func (e *StoreInUseError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeMemoryStoreInUse, "Memory store is used by a published agent version").WithCause(e)
}

// StoreAlreadyExistsError reports a store name already taken in the project.
type StoreAlreadyExistsError struct{ Name string }

func (e *StoreAlreadyExistsError) Error() string {
	return "memory: store name already taken: " + apperror.Echo(e.Name)
}

// IsStoreAlreadyExistsError reports whether err's tree contains a *StoreAlreadyExistsError.
func IsStoreAlreadyExistsError(err error) bool {
	_, ok := errors.AsType[*StoreAlreadyExistsError](err)
	return ok
}

// ToAppError maps StoreAlreadyExistsError to its own code with the name.
func (e *StoreAlreadyExistsError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeMemoryStoreAlreadyExists, "Memory store name already taken").
		WithMeta(map[string]string{"name": apperror.Echo(e.Name)}).WithCause(e)
}
