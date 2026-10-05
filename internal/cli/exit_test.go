package cli

import (
	"context"
	"errors"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/auth-server/apperror"
	apperrorv1 "altalune.id/auth-server/gen/go/apperror/v1"
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

func TestExitCodeFor_ConnectError(t *testing.T) {
	cerr := connect.NewError(connect.CodeNotFound, errors.New("Organization not found"))
	det, err := connect.NewErrorDetail(&apperrorv1.ErrorDetail{Code: "ORG001"})
	require.NoError(t, err)
	cerr.AddDetail(det)

	assert.Equal(t, ExitNotFound, ExitCodeFor(cerr))
	assert.Equal(t, ExitGeneral, ExitCodeFor(connect.NewError(connect.CodeNotFound, errors.New("x"))))
}
