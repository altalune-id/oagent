package cli

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	"altalune.id/oagent/apperror"
	apperrorv1 "altalune.id/oagent/gen/go/apperror/v1"
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
	if connect.CodeOf(err) == connect.CodeUnauthenticated {
		return ExitUnauth
	}
	return ExitGeneral
}

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
	switch apperror.KindOf(code) {
	case apperror.KindInvalid:
		return ExitInvalidArg
	case apperror.KindNotFound:
		return ExitNotFound
	case apperror.KindAlreadyExists:
		return ExitAlreadyExists
	case apperror.KindUnauthenticated:
		return ExitUnauth
	default:
		return ExitGeneral
	}
}
