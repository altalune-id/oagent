package controlplane

import (
	"connectrpc.com/connect"

	"altalune.id/oagent/apperror"
)

func connectCodeOf(code apperror.Code) connect.Code {
	switch apperror.KindOf(code) {
	case apperror.KindInvalid:
		return connect.CodeInvalidArgument
	case apperror.KindNotFound:
		return connect.CodeNotFound
	case apperror.KindAlreadyExists:
		return connect.CodeAlreadyExists
	case apperror.KindUnauthenticated:
		return connect.CodeUnauthenticated
	case apperror.KindForbidden:
		return connect.CodePermissionDenied
	case apperror.KindFailedPrecondition:
		return connect.CodeFailedPrecondition
	case apperror.KindResourceExhausted:
		return connect.CodeResourceExhausted
	case apperror.KindUnknown, apperror.KindInternal:
		return connect.CodeInternal
	default:
		return connect.CodeInternal
	}
}
