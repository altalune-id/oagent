package cli

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"altalune.id/auth-server/apperror"
)

func TestExitCodeFor(t *testing.T) {
	assert.Equal(t, ExitOK, ExitCodeFor(nil))
	assert.Equal(t, ExitOK, ExitCodeFor(context.Canceled))
	assert.Equal(t, ExitOK, ExitCodeFor(context.DeadlineExceeded))
	assert.Equal(t, ExitGeneral, ExitCodeFor(errors.New("plain")))
	assert.Equal(t, ExitInvalidArg, ExitCodeFor(apperror.New(apperror.CodeValidation, "x")))
	assert.Equal(t, ExitNotFound, ExitCodeFor(apperror.New(apperror.CodeOrgNotFound, "x")))
	assert.Equal(t, ExitAlreadyExists, ExitCodeFor(apperror.New(apperror.CodeOrgAlreadyExists, "x")))
	assert.Equal(t, ExitUnauth, ExitCodeFor(apperror.New(apperror.CodeTenantMissing, "x")))
	assert.Equal(t, ExitGeneral, ExitCodeFor(apperror.New(apperror.CodeUnexpected, "x")))
}

func TestRuntimeFromCtx_Missing(t *testing.T) {
	_, err := runtimeFromCtx(context.Background())
	assert.Error(t, err)
}
