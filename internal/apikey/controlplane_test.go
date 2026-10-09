package apikey_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	apikeyv1 "altalune.id/oagent/gen/go/apikey/v1"
	apikeyv1connect "altalune.id/oagent/gen/go/apikey/v1/apikeyv1connect"
	"altalune.id/oagent/internal/apikey"
	"altalune.id/oagent/internal/testutil/fakes"
)

func newAPIKeyClient(t *testing.T, store apikey.Store) apikeyv1connect.APIKeyServiceClient {
	t.Helper()
	svc := apikey.NewService(store, apikey.NewScheme(""), stubProjects{}, nil)
	_, handler := apikeyv1connect.NewAPIKeyServiceHandler(apikey.NewControlPlane(svc))
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	return apikeyv1connect.NewAPIKeyServiceClient(http.DefaultClient, ts.URL)
}

func TestAPIKeyService_Create(t *testing.T) {
	c := newAPIKeyClient(t, &fakes.APIKeyStore{})

	resp, err := c.CreateAPIKey(context.Background(), connect.NewRequest(&apikeyv1.CreateAPIKeyRequest{
		OrgId: uuid.New().String(), Kind: "org", Name: "deploy", AllProjects: true,
		ExpiresAt: timestamppb.New(time.Now().UTC().Add(time.Hour)),
	}))
	require.NoError(t, err)
	assert.NotEmpty(t, resp.Msg.GetPlaintext())
	assert.Equal(t, "org", resp.Msg.GetKey().GetKind())
	assert.NotEmpty(t, resp.Msg.GetKey().GetSecretHint())
}

func TestAPIKeyService_Create_InvalidOrg(t *testing.T) {
	c := newAPIKeyClient(t, &fakes.APIKeyStore{})

	_, err := c.CreateAPIKey(context.Background(), connect.NewRequest(&apikeyv1.CreateAPIKeyRequest{OrgId: "bad", Kind: "org", Name: "x", AllProjects: true, ExpiresAt: timestamppb.Now()}))
	assert.Error(t, err)
}

func TestAPIKeyService_List(t *testing.T) {
	k, _, err := apikey.NewScheme("").MintOrg(uuid.New(), "x", nil, apikey.ProjectGrant{All: true}, future(), time.Now().UTC())
	require.NoError(t, err)
	c := newAPIKeyClient(t, &fakes.APIKeyStore{Keys: []*apikey.APIKey{k}})

	resp, err := c.ListAPIKeys(context.Background(), connect.NewRequest(&apikeyv1.ListAPIKeysRequest{OrgId: k.OrgID.String()}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.GetKeys(), 1)
	assert.Equal(t, "x", resp.Msg.GetKeys()[0].GetName())
}

func TestAPIKeyService_List_InvalidOrg(t *testing.T) {
	c := newAPIKeyClient(t, &fakes.APIKeyStore{})

	_, err := c.ListAPIKeys(context.Background(), connect.NewRequest(&apikeyv1.ListAPIKeysRequest{OrgId: "bad"}))
	assert.Error(t, err)
}

func TestAPIKeyService_Revoke(t *testing.T) {
	orgID := uuid.New()
	k, _, err := apikey.NewScheme("").MintOrg(orgID, "x", nil, apikey.ProjectGrant{All: true}, future(), time.Now().UTC())
	require.NoError(t, err)
	c := newAPIKeyClient(t, &fakes.APIKeyStore{Key: k})

	_, err = c.RevokeAPIKey(context.Background(), connect.NewRequest(&apikeyv1.RevokeAPIKeyRequest{OrgId: orgID.String(), Id: k.ID.String()}))
	assert.NoError(t, err)

	_, err = c.RevokeAPIKey(context.Background(), connect.NewRequest(&apikeyv1.RevokeAPIKeyRequest{OrgId: orgID.String(), Id: "bad"}))
	assert.Error(t, err)
}

func TestAPIKeyService_Create_ProjectKey(t *testing.T) {
	c := newAPIKeyClient(t, &fakes.APIKeyStore{})

	resp, err := c.CreateAPIKey(context.Background(), connect.NewRequest(&apikeyv1.CreateAPIKeyRequest{
		OrgId: uuid.New().String(), ProjectId: uuid.New().String(), Kind: "project", Name: "ci",
		Scopes: []string{"agent:read"}, ResourceIds: []string{uuid.New().String()},
		ExpiresAt: timestamppb.New(time.Now().UTC().Add(time.Hour)),
	}))
	require.NoError(t, err)
	assert.Equal(t, "project", resp.Msg.GetKey().GetKind())
	assert.NotEmpty(t, resp.Msg.GetKey().GetProjectId())
	assert.Len(t, resp.Msg.GetKey().GetResourceIds(), 1)
}

func TestAPIKeyService_Create_BadUUIDs(t *testing.T) {
	c := newAPIKeyClient(t, &fakes.APIKeyStore{})

	_, err := c.CreateAPIKey(context.Background(), connect.NewRequest(&apikeyv1.CreateAPIKeyRequest{
		OrgId: uuid.New().String(), ProjectId: "bad", Kind: "project", Name: "ci", ExpiresAt: timestamppb.Now(),
	}))
	assert.Error(t, err)

	_, err = c.CreateAPIKey(context.Background(), connect.NewRequest(&apikeyv1.CreateAPIKeyRequest{
		OrgId: uuid.New().String(), Kind: "org", Name: "x", ProjectIds: []string{"bad"}, ExpiresAt: timestamppb.Now(),
	}))
	assert.Error(t, err)
}

func TestAPIKeyService_List_StoreError(t *testing.T) {
	c := newAPIKeyClient(t, &fakes.APIKeyStore{Err: assert.AnError})

	_, err := c.ListAPIKeys(context.Background(), connect.NewRequest(&apikeyv1.ListAPIKeysRequest{OrgId: uuid.New().String()}))
	assert.Error(t, err)
}

func TestAPIKeyService_Revoke_InvalidOrg(t *testing.T) {
	c := newAPIKeyClient(t, &fakes.APIKeyStore{})

	_, err := c.RevokeAPIKey(context.Background(), connect.NewRequest(&apikeyv1.RevokeAPIKeyRequest{OrgId: "bad", Id: uuid.New().String()}))
	assert.Error(t, err)
}
