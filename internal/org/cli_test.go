package org_test

import (
	"bytes"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	orgv1connect "altalune.id/oagent/gen/go/org/v1/orgv1connect"
	"altalune.id/oagent/internal/apiclient"
	"altalune.id/oagent/internal/controlplane"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/testutil/cptest"
	"altalune.id/oagent/internal/testutil/fakes"
)

func orgClient(t *testing.T, store org.Store) orgv1connect.OrgServiceClient {
	t.Helper()
	srv := controlplane.New(controlplane.Services{Orgs: org.NewService(store, fakes.Transactor{}, nil)}, nil, cptest.OperatorOptions())
	ts := httptest.NewServer(srv.Handler(""))
	t.Cleanup(ts.Close)
	return apiclient.New(ts.URL, cptest.OperatorCredential()).Org
}

func runOrg(t *testing.T, client orgv1connect.OrgServiceClient, args ...string) (*bytes.Buffer, error) {
	t.Helper()
	buf := new(bytes.Buffer)
	root := &cobra.Command{Use: "x", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().String("output", "", "")
	root.AddGroup(&cobra.Group{ID: "tenancy", Title: "Tenancy:"})
	root.AddCommand(org.NewCmd(func(*cobra.Command) (orgv1connect.OrgServiceClient, error) { return client, nil }))
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(append([]string{"org"}, args...))
	return buf, root.ExecuteContext(t.Context())
}

func TestOrgCreate(t *testing.T) {
	buf, err := runOrg(t, orgClient(t, &fakes.OrgStore{}), "create", "--name", "Acme", "--slug", "acme", "--output", "text")

	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Created org acme")
}

func TestOrgCreate_JSON(t *testing.T) {
	buf, err := runOrg(t, orgClient(t, &fakes.OrgStore{}), "create", "--name", "Acme", "--output", "json")

	require.NoError(t, err)
	assert.Contains(t, buf.String(), `"name": "Acme"`)
}

func TestOrgGet_JSON(t *testing.T) {
	store := &fakes.OrgStore{Org: &org.Org{ID: uuid.New(), PublicID: "pubpubpubpubpubpubpub", Slug: "acme", Name: "Acme"}}

	buf, err := runOrg(t, orgClient(t, store), "get", store.Org.PublicID, "--output", "json")

	require.NoError(t, err)
	assert.Contains(t, buf.String(), `"slug": "acme"`)
	assert.Contains(t, buf.String(), `"id": "pubpubpubpubpubpubpub"`)
	assert.NotContains(t, buf.String(), store.Org.ID.String())
}

func TestOrgGet_Text(t *testing.T) {
	store := &fakes.OrgStore{Org: &org.Org{ID: uuid.New(), PublicID: "pubpubpubpubpubpubpub", Slug: "acme", Name: "Acme"}}

	buf, err := runOrg(t, orgClient(t, store), "get", store.Org.PublicID, "--output", "text")

	require.NoError(t, err)
	assert.Contains(t, buf.String(), "SLUG")
	assert.NotContains(t, buf.String(), store.Org.ID.String())
}

func TestOrgGet_InvalidID(t *testing.T) {
	_, err := runOrg(t, orgClient(t, &fakes.OrgStore{}), "get", "not-a-uuid")
	assert.Error(t, err)
}

func TestOrgList_JSON(t *testing.T) {
	store := &fakes.OrgStore{Orgs: []*org.Org{
		{ID: uuid.New(), PublicID: "pubpubpubpubpubpubpub", Slug: "acme", Name: "Acme"},
	}}

	buf, err := runOrg(t, orgClient(t, store), "list", "--output", "json")

	require.NoError(t, err)
	assert.Contains(t, buf.String(), `"slug": "acme"`)
}

func TestOrgList_Text(t *testing.T) {
	store := &fakes.OrgStore{Orgs: []*org.Org{
		{ID: uuid.New(), PublicID: "pubpubpubpubpubpubpub", Slug: "acme", Name: "Acme"},
	}}

	buf, err := runOrg(t, orgClient(t, store), "list", "--output", "text")

	require.NoError(t, err)
	assert.Contains(t, buf.String(), "SLUG")
	assert.Contains(t, buf.String(), "acme")
}
