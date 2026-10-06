package controlplane

import (
	"connectrpc.com/connect"

	"altalune.id/oagent/apperror"
)

// connectCodeOf maps a stable apperror code onto the Connect status code clients see.
func connectCodeOf(code apperror.Code) connect.Code {
	switch code {
	case apperror.CodeTenantMissing:
		return connect.CodeUnauthenticated
	case apperror.CodeValidation, apperror.CodeOrgInvalidSlug, apperror.CodeOrgInvalidName:
		return connect.CodeInvalidArgument
	case apperror.CodeNotFound, apperror.CodeOrgNotFound:
		return connect.CodeNotFound
	case apperror.CodeAlreadyExists, apperror.CodeOrgAlreadyExists:
		return connect.CodeAlreadyExists
	case apperror.CodeUnexpected:
		return connect.CodeInternal
	default:
		return connect.CodeInternal
	}
}
