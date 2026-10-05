package cli

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	"altalune.id/auth-server/apperror"
	apperrorv1 "altalune.id/auth-server/gen/go/apperror/v1"
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
	if ae, ok := apperror.AsAppError(err); ok {
		return exitCode(ae.Code())
	}
	if code, ok := connectErrorCode(err); ok {
		return exitCode(code)
	}
	return ExitGeneral
}

// connectErrorCode extracts the apperror code from a remote connect.Error's typed detail.
func connectErrorCode(err error) (apperror.Code, bool) {
	cerr, ok := errors.AsType[*connect.Error](err)
	if !ok {
		return "", false
	}
	for _, d := range cerr.Details() {
		v, derr := d.Value()
		if derr != nil {
			continue
		}
		if det, dok := v.(*apperrorv1.ErrorDetail); dok {
			return apperror.Code(det.GetCode()), true
		}
	}
	return "", false
}

func exitCode(code apperror.Code) int {
	switch code {
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
