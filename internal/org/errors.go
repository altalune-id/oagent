package org

import (
	"errors"
	"fmt"
	"strconv"

	"altalune.id/oagent/apperror"
)

// NotFoundError signals a missing org row.
type NotFoundError struct {
	ID   string
	Slug string
}

func (e *NotFoundError) Error() string {
	switch {
	case e == nil:
		return "org: not found"
	case e.Slug != "":
		return fmt.Sprintf("org: not found: slug=%q", e.Slug)
	case e.ID != "":
		return fmt.Sprintf("org: not found: id=%s", e.ID)
	default:
		return "org: not found"
	}
}

// IsNotFoundError reports whether err wraps a *NotFoundError.
func IsNotFoundError(err error) bool {
	_, ok := errors.AsType[*NotFoundError](err)
	return ok
}

// AlreadyExistsError signals a slug collision on create.
type AlreadyExistsError struct {
	Slug string
}

func (e *AlreadyExistsError) Error() string {
	return fmt.Sprintf("org: slug already exists: %q", e.Slug)
}

// IsAlreadyExistsError reports whether err wraps an *AlreadyExistsError.
func IsAlreadyExistsError(err error) bool {
	_, ok := errors.AsType[*AlreadyExistsError](err)
	return ok
}

// SlugReason is the stable cause of an InvalidSlugError, sent to clients as meta.reason.
type SlugReason string

// The causes of an InvalidSlugError.
const (
	SlugLength   SlugReason = "length"
	SlugFormat   SlugReason = "format"
	SlugReserved SlugReason = "reserved"
)

// InvalidSlugError signals a slug that fails the aggregate invariants.
type InvalidSlugError struct {
	Slug   string
	Reason SlugReason
}

func (e *InvalidSlugError) Error() string {
	return "org: invalid slug: " + e.text()
}

func (e *InvalidSlugError) text() string {
	switch e.Reason {
	case SlugLength:
		return fmt.Sprintf("must be %d to %d characters", slugMinLen, slugMaxLen)
	case SlugFormat:
		return "must use lowercase letters, digits and dashes"
	case SlugReserved:
		return fmt.Sprintf("%q is reserved", apperror.Echo(e.Slug))
	default:
		return fmt.Sprintf("%q is invalid", apperror.Echo(e.Slug))
	}
}

// IsInvalidSlugError reports whether err wraps an *InvalidSlugError.
func IsInvalidSlugError(err error) bool {
	_, ok := errors.AsType[*InvalidSlugError](err)
	return ok
}

// NameReason is the stable cause of an InvalidNameError, sent to clients as meta.reason.
type NameReason string

// NameLength is the one cause of an InvalidNameError.
const NameLength NameReason = "length"

// InvalidNameError signals a name that fails the aggregate invariants.
type InvalidNameError struct {
	Reason NameReason
}

func (e *InvalidNameError) Error() string {
	return fmt.Sprintf("org: invalid name: must be %d to %d characters", nameMinLen, nameMaxLen)
}

// IsInvalidNameError reports whether err wraps an *InvalidNameError.
func IsInvalidNameError(err error) bool {
	_, ok := errors.AsType[*InvalidNameError](err)
	return ok
}

// CreationDisabledError reports an org create on a deployment that only has its system org.
type CreationDisabledError struct{}

func (*CreationDisabledError) Error() string { return "org: creation is disabled on this deployment" }

// ToAppError maps CreationDisabledError to its own code.
func (e *CreationDisabledError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeOrgCreationDisabled, "Creating organizations is disabled on this deployment").WithCause(e)
}

// IsCreationDisabledError reports whether err's tree contains a *CreationDisabledError.
func IsCreationDisabledError(err error) bool {
	_, ok := errors.AsType[*CreationDisabledError](err)
	return ok
}

// IsExpectedError reports whether err is a typed domain error a service returns as-is.
func IsExpectedError(err error) bool {
	return IsNotFoundError(err) || IsAlreadyExistsError(err) || IsInvalidSlugError(err) || IsInvalidNameError(err) ||
		IsCreationDisabledError(err)
}

// ToAppError maps NotFoundError to the canonical NotFound envelope.
func (e *NotFoundError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeOrgNotFound, "Organization not found").WithCause(e)
}

// ToAppError maps AlreadyExistsError to the canonical AlreadyExists envelope.
func (e *AlreadyExistsError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeOrgAlreadyExists, "Organization slug already taken").
		WithMeta(map[string]string{"slug": apperror.Echo(e.Slug)}).WithCause(e)
}

// ToAppError maps InvalidSlugError to its own code with the slug, the reason and the length limits.
func (e *InvalidSlugError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeOrgInvalidSlug, "Organization slug "+e.text()).
		WithMeta(map[string]string{
			"slug": apperror.Echo(e.Slug), "reason": string(e.Reason),
			"min": strconv.Itoa(slugMinLen), "max": strconv.Itoa(slugMaxLen),
		}).WithCause(e)
}

// ToAppError maps InvalidNameError to its own code with the reason and the length limits.
func (e *InvalidNameError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeOrgInvalidName, fmt.Sprintf("Organization name must be %d to %d characters", nameMinLen, nameMaxLen)).
		WithMeta(map[string]string{"reason": string(e.Reason), "min": strconv.Itoa(nameMinLen), "max": strconv.Itoa(nameMaxLen)}).WithCause(e)
}
