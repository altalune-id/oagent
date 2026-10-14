package agent

import (
	"errors"
	"fmt"
	"strconv"

	"altalune.id/oagent/apperror"
)

// NotFoundError signals a missing agent row.
type NotFoundError struct {
	ID   string
	Slug string
}

func (e *NotFoundError) Error() string {
	switch {
	case e == nil:
		return "agent: not found"
	case e.Slug != "":
		return fmt.Sprintf("agent: not found: slug=%q", e.Slug)
	case e.ID != "":
		return fmt.Sprintf("agent: not found: id=%s", e.ID)
	default:
		return "agent: not found"
	}
}

// IsNotFoundError reports whether err wraps a *NotFoundError.
func IsNotFoundError(err error) bool {
	_, ok := errors.AsType[*NotFoundError](err)
	return ok
}

// AlreadyExistsError signals a slug collision within a project.
type AlreadyExistsError struct {
	Slug string
}

func (e *AlreadyExistsError) Error() string {
	return fmt.Sprintf("agent: slug already exists: %q", e.Slug)
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
	SlugLength SlugReason = "length"
	SlugFormat SlugReason = "format"
)

// InvalidSlugError signals a slug that fails the aggregate invariants.
type InvalidSlugError struct {
	Slug   string
	Reason SlugReason
}

func (e *InvalidSlugError) Error() string {
	return "agent: invalid slug: " + e.text()
}

func (e *InvalidSlugError) text() string {
	switch e.Reason {
	case SlugLength:
		return fmt.Sprintf("must be %d to %d characters", slugMinLen, slugMaxLen)
	case SlugFormat:
		return "must use lowercase letters, digits and dashes"
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
	return fmt.Sprintf("agent: invalid name: must be %d to %d characters", nameMinLen, nameMaxLen)
}

// IsInvalidNameError reports whether err wraps an *InvalidNameError.
func IsInvalidNameError(err error) bool {
	_, ok := errors.AsType[*InvalidNameError](err)
	return ok
}

// DescriptionReason is the stable cause of an InvalidDescriptionError, sent to clients as meta.reason.
type DescriptionReason string

// DescriptionLength is the one cause of an InvalidDescriptionError.
const DescriptionLength DescriptionReason = "length"

// InvalidDescriptionError signals a description that fails the aggregate invariants.
type InvalidDescriptionError struct {
	Reason DescriptionReason
}

func (e *InvalidDescriptionError) Error() string {
	return fmt.Sprintf("agent: invalid description: must be at most %d characters", descMaxLen)
}

// IsInvalidDescriptionError reports whether err wraps an *InvalidDescriptionError.
func IsInvalidDescriptionError(err error) bool {
	_, ok := errors.AsType[*InvalidDescriptionError](err)
	return ok
}

// IsExpectedError reports whether err is a typed domain error a service returns as-is.
func IsExpectedError(err error) bool {
	return IsNotFoundError(err) || IsAlreadyExistsError(err) || IsInvalidSlugError(err) ||
		IsInvalidNameError(err) || IsInvalidDescriptionError(err)
}

// ToAppError maps NotFoundError to the canonical NotFound envelope.
func (e *NotFoundError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeAgentNotFound, "Agent not found").WithCause(e)
}

// ToAppError maps AlreadyExistsError to the canonical AlreadyExists envelope.
func (e *AlreadyExistsError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeAgentAlreadyExists, "Agent slug already taken").
		WithMeta(map[string]string{"slug": apperror.Echo(e.Slug)}).WithCause(e)
}

// ToAppError maps InvalidSlugError to its own code with the slug, the reason and the length limits.
func (e *InvalidSlugError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeAgentInvalidSlug, "Agent slug "+e.text()).
		WithMeta(map[string]string{
			"slug": apperror.Echo(e.Slug), "reason": string(e.Reason),
			"min": strconv.Itoa(slugMinLen), "max": strconv.Itoa(slugMaxLen),
		}).WithCause(e)
}

// ToAppError maps InvalidNameError to its own code with the reason and the length limits.
func (e *InvalidNameError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeAgentInvalidName, fmt.Sprintf("Agent name must be %d to %d characters", nameMinLen, nameMaxLen)).
		WithMeta(map[string]string{"reason": string(e.Reason), "min": strconv.Itoa(nameMinLen), "max": strconv.Itoa(nameMaxLen)}).WithCause(e)
}

// ToAppError maps InvalidDescriptionError to its own code with the reason and the length limit.
func (e *InvalidDescriptionError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeAgentInvalidDescription, fmt.Sprintf("Agent description must be at most %d characters", descMaxLen)).
		WithMeta(map[string]string{"reason": string(e.Reason), "max": strconv.Itoa(descMaxLen)}).WithCause(e)
}
