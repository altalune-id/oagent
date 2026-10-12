package controlplane

import (
	"context"
	"errors"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	apperrorv1 "altalune.id/oagent/gen/go/apperror/v1"
	orgv1 "altalune.id/oagent/gen/go/org/v1"
	"altalune.id/oagent/internal/webhook"
	"altalune.id/oagent/reqid"
)

func TestTranslate_AppError(t *testing.T) {
	ctx := reqid.WithContext(context.Background(), "req-1")
	err := Translate(ctx, apperror.New(apperror.CodeOrgNotFound, "nope").WithMeta(map[string]string{"slug": "x"}), nil)

	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	var cerr *connect.Error
	require.ErrorAs(t, err, &cerr)
	require.Len(t, cerr.Details(), 1)
	value, verr := cerr.Details()[0].Value()
	require.NoError(t, verr)
	det, ok := value.(*apperrorv1.ErrorDetail)
	require.True(t, ok)
	assert.Equal(t, "ORG001", det.GetCode())
	assert.Equal(t, "x", det.GetMeta()["slug"])
	assert.Equal(t, "req-1", det.GetRequestId())
}

func TestTranslate_UnexpectedWithoutReporter(t *testing.T) {
	err := Translate(context.Background(), errors.New("boom"), nil)
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
}

func TestTranslate_UnexpectedWithReporter(t *testing.T) {
	called := false
	unexpected := func(context.Context, string, error, ...any) *apperror.AppError {
		called = true
		return apperror.New(apperror.CodeUnexpected, "reported")
	}
	err := Translate(context.Background(), errors.New("boom"), unexpected)
	assert.True(t, called)
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
}

func TestWrap_Passthrough(t *testing.T) {
	wrapped := Wrap(nil)(func(context.Context, connect.AnyRequest) (connect.AnyResponse, error) {
		return connect.NewResponse(&orgv1.GetOrgResponse{}), nil
	})
	resp, err := wrapped(context.Background(), connect.NewRequest(&orgv1.GetOrgRequest{}))
	require.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestTranslate_WebhookValidationIsNotAnIncident(t *testing.T) {
	called := false
	unexpected := func(context.Context, string, error, ...any) *apperror.AppError {
		called = true
		return apperror.New(apperror.CodeUnexpected, "reported")
	}
	err := Translate(context.Background(), &webhook.InvalidURLError{Reason: webhook.URLHTTPSRequired}, unexpected)

	assert.False(t, called)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	var cerr *connect.Error
	require.ErrorAs(t, err, &cerr)
	value, verr := cerr.Details()[0].Value()
	require.NoError(t, verr)
	det, ok := value.(*apperrorv1.ErrorDetail)
	require.True(t, ok)
	assert.Equal(t, "WHK002", det.GetCode())
	assert.Equal(t, "https_required", det.GetMeta()["reason"])
}
