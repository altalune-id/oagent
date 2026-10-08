package project

import (
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"

	"altalune.id/oagent/apperror"
)

// NotFoundError signals a missing project row.
type NotFoundError struct {
	ID   string
	Slug string
}

func (e *NotFoundError) Error() string {
	switch {
	case e == nil:
		return "project: not found"
	case e.Slug != "":
		return fmt.Sprintf("project: not found: slug=%q", e.Slug)
	case e.ID != "":
		return fmt.Sprintf("project: not found: id=%s", e.ID)
	default:
		return "project: not found"
	}
}

// IsNotFoundError reports whether err wraps a *NotFoundError.
func IsNotFoundError(err error) bool {
	_, ok := errors.AsType[*NotFoundError](err)
	return ok
}

// AlreadyExistsError signals a slug collision within an organization.
type AlreadyExistsError struct {
	Slug string
}

func (e *AlreadyExistsError) Error() string {
	return fmt.Sprintf("project: slug already exists: %q", e.Slug)
}

// IsAlreadyExistsError reports whether err wraps an *AlreadyExistsError.
func IsAlreadyExistsError(err error) bool {
	_, ok := errors.AsType[*AlreadyExistsError](err)
	return ok
}

// InvalidSlugError signals a slug that fails the aggregate invariants.
type InvalidSlugError struct {
	Slug   string
	Reason string
}

func (e *InvalidSlugError) Error() string {
	if e.Reason == "" {
		return fmt.Sprintf("project: invalid slug: %q", e.Slug)
	}
	return fmt.Sprintf("project: invalid slug: %s", e.Reason)
}

// IsInvalidSlugError reports whether err wraps an *InvalidSlugError.
func IsInvalidSlugError(err error) bool {
	_, ok := errors.AsType[*InvalidSlugError](err)
	return ok
}

// InvalidNameError signals a name that fails the aggregate invariants.
type InvalidNameError struct {
	Reason string
}

func (e *InvalidNameError) Error() string {
	if e.Reason == "" {
		return "project: invalid name"
	}
	return "project: invalid name: " + e.Reason
}

// IsInvalidNameError reports whether err wraps an *InvalidNameError.
func IsInvalidNameError(err error) bool {
	_, ok := errors.AsType[*InvalidNameError](err)
	return ok
}

// IsExpectedError reports whether err is a typed domain error a service returns as-is.
func IsExpectedError(err error) bool {
	return IsNotFoundError(err) || IsAlreadyExistsError(err) || IsInvalidSlugError(err) || IsInvalidNameError(err)
}

// ToAppError maps NotFoundError to the canonical NotFound envelope.
func (e *NotFoundError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeProjectNotFound, "Project not found").WithCause(e)
}

// ToAppError maps AlreadyExistsError to the canonical AlreadyExists envelope.
func (e *AlreadyExistsError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeProjectAlreadyExists, "Project slug already taken").WithCause(e)
}

// ToAppError maps InvalidSlugError to the canonical Validation envelope.
func (e *InvalidSlugError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeProjectInvalidSlug, "Invalid project slug").WithCause(e)
}

// ToAppError maps InvalidNameError to the canonical Validation envelope.
func (e *InvalidNameError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeProjectInvalidName, "Invalid project name").WithCause(e)
}

// IsUniqueViolation reports whether err is a Postgres unique-violation (SQLSTATE 23505).
func IsUniqueViolation(err error) bool {
	pgErr, ok := errors.AsType[*pgconn.PgError](err)
	return ok && pgErr.Code == "23505"
}
