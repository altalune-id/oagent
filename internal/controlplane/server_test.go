package controlplane

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	orgv1 "altalune.id/oagent/gen/go/org/v1"
	orgv1connect "altalune.id/oagent/gen/go/org/v1/orgv1connect"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/testutil/fakes"
)

func TestHandler_MountsUnderBasePathAndTranslatesErrors(t *testing.T) {
	srv := New(Services{Orgs: org.NewService(&fakes.OrgStore{}, fakes.Transactor{}, nil)}, nil, Options{})
	ts := httptest.NewServer(srv.Handler("/app"))
	t.Cleanup(ts.Close)
	c := orgv1connect.NewOrgServiceClient(http.DefaultClient, ts.URL+"/app/api")

	resp, err := c.CreateOrg(context.Background(), connect.NewRequest(&orgv1.CreateOrgRequest{Slug: "acme", Name: "Acme"}))
	require.NoError(t, err)
	assert.Equal(t, "acme", resp.Msg.GetOrg().GetSlug())

	_, err = c.CreateOrg(context.Background(), connect.NewRequest(&orgv1.CreateOrgRequest{Slug: "A", Name: "Acme"}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestRecoverPanic(t *testing.T) {
	s := New(Services{Orgs: org.NewService(&fakes.OrgStore{}, fakes.Transactor{}, nil)}, nil, Options{})
	err := s.recoverPanic(context.Background(), connect.Spec{}, nil, "boom")
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
}

func TestRecoverHTTP(t *testing.T) {
	s := New(Services{Orgs: org.NewService(&fakes.OrgStore{}, fakes.Transactor{}, nil)}, nil, Options{})
	h := s.recoverHTTP(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}
