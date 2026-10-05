package cli

import (
	"context"
	"errors"

	"altalune.id/auth-server/apperror"
)

const (
	ExitOK            = 0
	ExitGeneral       = 1
	ExitInvalidArg    = 2
	ExitNotFound      = 3
	ExitAlreadyExists = 4
	ExitUnauth        = 5
)

// ExitCodeFor maps err to a stable process exit code; unknown errors return 1.
func ExitCodeFor(err error) int {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return ExitOK
	}
	ae, ok := apperror.AsAppError(err)
	if !ok {
		return ExitGeneral
	}
	switch ae.Code() {
	case apperror.CodeValidation, apperror.CodeOrgInvalidSlug, apperror.CodeOrgInvalidName:
		return ExitInvalidArg
	case apperror.CodeNotFound, apperror.CodeOrgNotFound:
		return ExitNotFound
	case apperror.CodeAlreadyExists, apperror.CodeOrgAlreadyExists:
		return ExitAlreadyExists
	case apperror.CodeTenantMissing:
		return ExitUnauth
	default:
		return ExitGeneral
	}
}
