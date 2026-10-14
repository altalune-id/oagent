package apperror

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKindOf(t *testing.T) {
	cases := map[Code]Kind{
		CodeAgentNotFound:          KindNotFound,
		CodeAPIKeyPersonOnlyScope:  KindInvalid,
		CodeForbidden:              KindForbidden,
		CodeUnauthenticated:        KindUnauthenticated,
		CodeOrgCreationDisabled:    KindFailedPrecondition,
		CodeInstallAlreadyDone:     KindAlreadyExists,
		CodeAuthInvalidCredentials: KindUnauthenticated,
		CodeAuthRateLimited:        KindResourceExhausted,
		CodeUnexpected:             KindInternal,
		Code("BOGUS"):              KindInternal,
	}
	assert.Equal(t, KindInternal+1, KindResourceExhausted, "new kinds go last so no existing value shifts")
	for code, want := range cases {
		t.Run(string(code), func(t *testing.T) {
			assert.Equal(t, want, KindOf(code))
		})
	}
}
