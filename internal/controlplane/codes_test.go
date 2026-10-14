package controlplane

import (
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"

	"altalune.id/oagent/apperror"
)

func TestConnectCodeOf(t *testing.T) {
	cases := map[apperror.Code]connect.Code{
		apperror.CodeTenantMissing:    connect.CodeUnauthenticated,
		apperror.CodeValidation:       connect.CodeInvalidArgument,
		apperror.CodeOrgInvalidSlug:   connect.CodeInvalidArgument,
		apperror.CodeOrgInvalidName:   connect.CodeInvalidArgument,
		apperror.CodeNotFound:         connect.CodeNotFound,
		apperror.CodeOrgNotFound:      connect.CodeNotFound,
		apperror.CodeAlreadyExists:    connect.CodeAlreadyExists,
		apperror.CodeOrgAlreadyExists: connect.CodeAlreadyExists,
		apperror.CodeUnexpected:       connect.CodeInternal,
		apperror.Code("BOGUS"):        connect.CodeInternal,

		apperror.CodeAgentNotFound:          connect.CodeNotFound,
		apperror.CodeAPIKeyInvalidScope:     connect.CodeInvalidArgument,
		apperror.CodeProjectAlreadyExists:   connect.CodeAlreadyExists,
		apperror.CodeUnauthenticated:        connect.CodeUnauthenticated,
		apperror.CodeAuthInvalidCredentials: connect.CodeUnauthenticated,
		apperror.CodeForbidden:              connect.CodePermissionDenied,
		apperror.CodeOrgCreationDisabled:    connect.CodeFailedPrecondition,
		apperror.CodeAuthRateLimited:        connect.CodeResourceExhausted,
	}
	for code, want := range cases {
		t.Run(string(code), func(t *testing.T) {
			assert.Equal(t, want, connectCodeOf(code))
		})
	}
}
