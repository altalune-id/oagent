package authz

import (
	"errors"
	"fmt"

	"altalune.id/oagent/apperror"
)

// ForbiddenError signals a Principal without the required permission.
type ForbiddenError struct{ Permission string }

func (e *ForbiddenError) Error() string {
	return fmt.Sprintf("authz: forbidden: permission %q", e.Permission)
}

// IsForbiddenError reports whether err wraps a *ForbiddenError.
func IsForbiddenError(err error) bool {
	_, ok := errors.AsType[*ForbiddenError](err)
	return ok
}

// ToAppError maps ForbiddenError to the canonical Forbidden envelope.
func (e *ForbiddenError) ToAppError() *apperror.AppError {
	ae := apperror.New(apperror.CodeForbidden, "You do not have permission to do this").WithCause(e)
	if e.Permission == "" {
		return ae
	}
	return ae.WithMeta(map[string]string{"permission": e.Permission})
}
