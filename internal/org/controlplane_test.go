package org_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	orgv1 "altalune.id/oagent/gen/go/org/v1"
	orgv1connect "altalune.id/oagent/gen/go/org/v1/orgv1connect"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/testutil/fakes"
)

func newOrgServiceClient(t *testing.T, store org.Store) orgv1connect.OrgServiceClient {
	t.Helper()
	_, handler := orgv1connect.NewOrgServiceHandler(org.NewControlPlane(org.NewService(store, fakes.Transactor{}, nil)),
		connect.WithInterceptors(fakes.OperatorInterceptor()))
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	return orgv1connect.NewOrgServiceClient(http.DefaultClient, ts.URL)
}

func TestOrgService_CreateOrg(t *testing.T) {
	c := newOrgServiceClient(t, &fakes.OrgStore{})

	resp, err := c.CreateOrg(context.Background(), connect.NewRequest(&orgv1.CreateOrgRequest{Slug: "acme", Name: "Acme"}))
	require.NoError(t, err)
	assert.Equal(t, "acme", resp.Msg.GetOrg().GetSlug())
	assert.Equal(t, "Acme", resp.Msg.GetOrg().GetName())
	assert.Len(t, resp.Msg.GetOrg().GetId(), 21)
	assert.NotNil(t, resp.Msg.GetOrg().GetCreatedAt())
}

func TestOrgService_CreateOrg_InvalidSlug(t *testing.T) {
	c := newOrgServiceClient(t, &fakes.OrgStore{})

	_, err := c.CreateOrg(context.Background(), connect.NewRequest(&orgv1.CreateOrgRequest{Slug: "A", Name: "Acme"}))
	assert.Error(t, err)
}

func TestOrgService_GetOrg(t *testing.T) {
	store := &fakes.OrgStore{Org: &org.Org{ID: uuid.New(), PublicID: "pub", Slug: "acme", Name: "Acme"}}
	c := newOrgServiceClient(t, store)

	resp, err := c.GetOrg(context.Background(), connect.NewRequest(&orgv1.GetOrgRequest{Id: "pub"}))
	require.NoError(t, err)
	assert.Equal(t, "acme", resp.Msg.GetOrg().GetSlug())
	assert.Equal(t, "pub", resp.Msg.GetOrg().GetId())
}

func TestOrgService_GetOrg_UnknownID(t *testing.T) {
	c := newOrgServiceClient(t, &fakes.OrgStore{Err: &org.NotFoundError{}})

	_, err := c.GetOrg(context.Background(), connect.NewRequest(&orgv1.GetOrgRequest{Id: "not-a-public-id"}))
	assert.Error(t, err)
}

func TestOrgService_GetOrg_StoreError(t *testing.T) {
	c := newOrgServiceClient(t, &fakes.OrgStore{Err: &org.NotFoundError{Slug: "acme"}})

	_, err := c.GetOrg(context.Background(), connect.NewRequest(&orgv1.GetOrgRequest{Id: "pub"}))
	assert.Error(t, err)
}

func TestOrgService_ListOrgs(t *testing.T) {
	store := &fakes.OrgStore{Orgs: []*org.Org{
		{ID: uuid.New(), PublicID: "pub1", Slug: "acme", Name: "Acme"},
		{ID: uuid.New(), PublicID: "pub2", Slug: "globex", Name: "Globex"},
	}}
	c := newOrgServiceClient(t, store)

	resp, err := c.ListOrgs(context.Background(), connect.NewRequest(&orgv1.ListOrgsRequest{}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.GetOrgs(), 2)
	assert.Equal(t, "acme", resp.Msg.GetOrgs()[0].GetSlug())
	assert.Equal(t, "globex", resp.Msg.GetOrgs()[1].GetSlug())
	assert.Equal(t, "pub1", resp.Msg.GetOrgs()[0].GetId())
}

func TestOrgService_ListOrgs_StoreError(t *testing.T) {
	c := newOrgServiceClient(t, &fakes.OrgStore{Err: &org.NotFoundError{Slug: "acme"}})

	_, err := c.ListOrgs(context.Background(), connect.NewRequest(&orgv1.ListOrgsRequest{}))
	assert.Error(t, err)
}
