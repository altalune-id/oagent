package controlplane

import (
	"context"
	"errors"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	orgv1 "altalune.id/oagent/gen/go/org/v1"
	"altalune.id/oagent/reqid"
)

func TestRequestID_MintsAndSetsHeader(t *testing.T) {
	ic := RequestID()
	next := func(ctx context.Context, _ connect.AnyRequest) (connect.AnyResponse, error) {
		assert.NotEmpty(t, reqid.FromContext(ctx))
		return connect.NewResponse(&orgv1.GetOrgResponse{}), nil
	}
	resp, err := ic.WrapUnary(next)(context.Background(), connect.NewRequest(&orgv1.GetOrgRequest{}))
	require.NoError(t, err)
	assert.NotEmpty(t, resp.Header().Get(reqid.Header))
}

func TestRequestID_PropagatesInbound(t *testing.T) {
	ic := RequestID()
	next := func(ctx context.Context, _ connect.AnyRequest) (connect.AnyResponse, error) {
		assert.Equal(t, "inbound-1", reqid.FromContext(ctx))
		return connect.NewResponse(&orgv1.GetOrgResponse{}), nil
	}
	req := connect.NewRequest(&orgv1.GetOrgRequest{})
	req.Header().Set(reqid.Header, "inbound-1")

	resp, err := ic.WrapUnary(next)(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "inbound-1", resp.Header().Get(reqid.Header))
}

func TestRequestID_ErrorMeta(t *testing.T) {
	ic := RequestID()
	next := func(context.Context, connect.AnyRequest) (connect.AnyResponse, error) {
		return nil, connect.NewError(connect.CodeInternal, errors.New("x"))
	}
	_, err := ic.WrapUnary(next)(context.Background(), connect.NewRequest(&orgv1.GetOrgRequest{}))

	var cerr *connect.Error
	require.ErrorAs(t, err, &cerr)
	assert.NotEmpty(t, cerr.Meta().Get(reqid.Header))
}
