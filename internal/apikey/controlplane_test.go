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
	"altalune.id/oagent/internal/controlplane"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/testutil/fakes"
)

func newAPIKeyClient(t *testing.T, store apikey.Store, refs *fakes.PublicIDs) apikeyv1connect.APIKeyServiceClient {
	t.Helper()
	svc := apikey.NewService(store, apikey.NewScheme(""), stubProjects{ids: []uuid.UUID{knownProject}}, testUnexpected())
	_, handler := apikeyv1connect.NewAPIKeyServiceHandler(apikey.NewControlPlane(svc, refs), connect.WithInterceptors(controlplane.Wrap(nil), fakes.OperatorInterceptor()))
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	return apikeyv1connect.NewAPIKeyServiceClient(http.DefaultClient, ts.URL)
}

func TestAPIKeyService_Create(t *testing.T) {
	refs := fakes.NewPublicIDs()
	orgPub := refs.Org(uuid.New())
	c := newAPIKeyClient(t, &fakes.APIKeyStore{}, refs)

	resp, err := c.CreateAPIKey(context.Background(), connect.NewRequest(&apikeyv1.CreateAPIKeyRequest{
		OrgId: orgPub, Kind: "org", Name: "deploy", AllProjects: true,
		ExpiresAt: timestamppb.New(time.Now().UTC().Add(time.Hour)),
	}))
	require.NoError(t, err)
	assert.NotEmpty(t, resp.Msg.GetPlaintext())
	assert.Equal(t, "org", resp.Msg.GetKey().GetKind())
	assert.Equal(t, orgPub, resp.Msg.GetKey().GetOrgId())
	assert.NotEmpty(t, resp.Msg.GetKey().GetSecretHint())
}

// SECURITY: an unknown, malformed or UUID-shaped org id is the same not-found.
func TestAPIKeyService_Create_UnknownOrg(t *testing.T) {
	refs := fakes.NewPublicIDs()
	orgID := uuid.New()
	refs.Org(orgID)
	c := newAPIKeyClient(t, &fakes.APIKeyStore{}, refs)

	for _, org := range []string{"bad", orgID.String(), "AAAAAAAAAAAAAAAAAAAAA"} {
		_, err := c.CreateAPIKey(context.Background(), connect.NewRequest(&apikeyv1.CreateAPIKeyRequest{OrgId: org, Kind: "org", Name: "x", AllProjects: true, ExpiresAt: timestamppb.Now()}))
		assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err), org)
	}
}

func TestAPIKeyService_List(t *testing.T) {
	k, _, err := apikey.NewScheme("").MintOrg(uuid.New(), "x", nil, apikey.ProjectGrant{All: true}, future(), time.Now().UTC())
	require.NoError(t, err)
	refs := fakes.NewPublicIDs()
	orgPub := refs.Org(k.OrgID)
	c := newAPIKeyClient(t, &fakes.APIKeyStore{Keys: []*apikey.APIKey{k}}, refs)

	resp, err := c.ListAPIKeys(context.Background(), connect.NewRequest(&apikeyv1.ListAPIKeysRequest{OrgId: orgPub}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.GetKeys(), 1)
	assert.Equal(t, "x", resp.Msg.GetKeys()[0].GetName())
	assert.Equal(t, k.SecretHint, resp.Msg.GetKeys()[0].GetSecretHint())
	assert.Equal(t, orgPub, resp.Msg.GetKeys()[0].GetOrgId())
}

func TestAPIKeyService_List_UnknownOrg(t *testing.T) {
	refs := fakes.NewPublicIDs()
	orgID := uuid.New()
	refs.Org(orgID)
	c := newAPIKeyClient(t, &fakes.APIKeyStore{}, refs)

	for _, org := range []string{"bad", orgID.String()} {
		_, err := c.ListAPIKeys(context.Background(), connect.NewRequest(&apikeyv1.ListAPIKeysRequest{OrgId: org}))
		assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err), org)
	}
}

func TestAPIKeyService_Revoke(t *testing.T) {
	orgID := uuid.New()
	k, _, err := apikey.NewScheme("").MintOrg(orgID, "x", nil, apikey.ProjectGrant{All: true}, future(), time.Now().UTC())
	require.NoError(t, err)
	refs := fakes.NewPublicIDs()
	orgPub := refs.Org(orgID)
	store := &fakes.APIKeyStore{Key: k}
	c := newAPIKeyClient(t, store, refs)

	_, err = c.RevokeAPIKey(context.Background(), connect.NewRequest(&apikeyv1.RevokeAPIKeyRequest{OrgId: orgPub, Hint: k.SecretHint}))
	require.NoError(t, err)
	require.NotNil(t, store.Saved)
	assert.NotNil(t, store.Saved.RevokedAt)

	_, err = c.RevokeAPIKey(context.Background(), connect.NewRequest(&apikeyv1.RevokeAPIKeyRequest{OrgId: orgPub, Hint: "zzzz"}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

// SECURITY: a hint names a key only within the org asked for.
func TestAPIKeyService_Revoke_OtherOrgsHint(t *testing.T) {
	orgA, orgB := uuid.New(), uuid.New()
	k, _, err := apikey.NewScheme("").MintOrg(orgB, "x", nil, apikey.ProjectGrant{All: true}, future(), time.Now().UTC())
	require.NoError(t, err)
	refs := fakes.NewPublicIDs()
	pubA := refs.Org(orgA)
	refs.Org(orgB)
	store := &fakes.APIKeyStore{Key: k}

	_, err = newAPIKeyClient(t, store, refs).RevokeAPIKey(context.Background(), connect.NewRequest(&apikeyv1.RevokeAPIKeyRequest{OrgId: pubA, Hint: k.SecretHint}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	assert.Nil(t, store.Saved)
}

func TestAPIKeyService_Create_ProjectKey(t *testing.T) {
	refs := fakes.NewPublicIDs()
	orgID := uuid.New()
	orgPub := refs.Org(orgID)
	projectPub := refs.Project(knownProject, orgID)
	resourcePub := refs.Resource(uuid.New(), orgID)
	store := &fakes.APIKeyStore{}
	c := newAPIKeyClient(t, store, refs)

	resp, err := c.CreateAPIKey(context.Background(), connect.NewRequest(&apikeyv1.CreateAPIKeyRequest{
		OrgId: orgPub, ProjectId: projectPub, Kind: "project", Name: "ci",
		Scopes: []string{"agent:read"}, ResourceIds: []string{resourcePub},
		ExpiresAt: timestamppb.New(time.Now().UTC().Add(time.Hour)),
	}))
	require.NoError(t, err)
	assert.Equal(t, "project", resp.Msg.GetKey().GetKind())
	assert.Equal(t, projectPub, resp.Msg.GetKey().GetProjectId())
	assert.Equal(t, []string{resourcePub}, resp.Msg.GetKey().GetResourceIds())
	assert.Equal(t, knownProject, store.Saved.ProjectID)
}

// SECURITY: an unknown, UUID-shaped or another org's project is a grant outside the org.
func TestAPIKeyService_Create_ProjectOutsideOrg(t *testing.T) {
	refs := fakes.NewPublicIDs()
	orgID := uuid.New()
	orgPub := refs.Org(orgID)
	foreign := refs.Project(knownProject, uuid.New())

	for _, project := range []string{"bad", knownProject.String(), foreign} {
		store := &fakes.APIKeyStore{}
		c := newAPIKeyClient(t, store, refs)
		_, err := c.CreateAPIKey(context.Background(), connect.NewRequest(&apikeyv1.CreateAPIKeyRequest{
			OrgId: orgPub, ProjectId: project, Kind: "project", Name: "ci", ExpiresAt: timestamppb.Now(),
		}))
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err), project)

		_, err = c.CreateAPIKey(context.Background(), connect.NewRequest(&apikeyv1.CreateAPIKeyRequest{
			OrgId: orgPub, Kind: "org", Name: "x", ProjectIds: []string{project}, ExpiresAt: timestamppb.Now(),
		}))
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err), project)
		assert.Nil(t, store.Saved, project)
	}
}

// SECURITY: an unknown, UUID-shaped or another org's resource is refused.
func TestAPIKeyService_Create_UnknownResource(t *testing.T) {
	refs := fakes.NewPublicIDs()
	orgID := uuid.New()
	orgPub := refs.Org(orgID)
	projectPub := refs.Project(knownProject, orgID)
	foreignID := uuid.New()
	foreign := refs.Resource(foreignID, uuid.New())

	for _, res := range []string{"bad", foreignID.String(), foreign} {
		store := &fakes.APIKeyStore{}
		_, err := newAPIKeyClient(t, store, refs).CreateAPIKey(context.Background(), connect.NewRequest(&apikeyv1.CreateAPIKeyRequest{
			OrgId: orgPub, ProjectId: projectPub, Kind: "project", Name: "ci",
			Scopes: []string{"agent:read"}, ResourceIds: []string{res}, ExpiresAt: timestamppb.New(time.Now().UTC().Add(time.Hour)),
		}))
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err), res)
		assert.Nil(t, store.Saved, res)
	}
}

func TestAPIKeyService_List_StoreError(t *testing.T) {
	refs := fakes.NewPublicIDs()
	c := newAPIKeyClient(t, &fakes.APIKeyStore{Err: assert.AnError}, refs)

	_, err := c.ListAPIKeys(context.Background(), connect.NewRequest(&apikeyv1.ListAPIKeysRequest{OrgId: refs.Org(uuid.New())}))
	assert.Error(t, err)
}

func TestAPIKeyService_Revoke_UnknownOrg(t *testing.T) {
	orgID := uuid.New()
	k, _, err := apikey.NewScheme("").MintOrg(orgID, "x", nil, apikey.ProjectGrant{All: true}, future(), time.Now().UTC())
	require.NoError(t, err)
	refs := fakes.NewPublicIDs()
	refs.Org(orgID)
	store := &fakes.APIKeyStore{Key: k}
	c := newAPIKeyClient(t, store, refs)

	for _, org := range []string{"bad", orgID.String()} {
		_, err := c.RevokeAPIKey(context.Background(), connect.NewRequest(&apikeyv1.RevokeAPIKeyRequest{OrgId: org, Hint: k.SecretHint}))
		assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err), org)
	}
	assert.Nil(t, store.Saved)
}

// Review Focus 5: listing an org's keys is org-wide, so a key limited to some projects is refused.
func TestAPIKeyService_List_ProjectLimitedKeyRefused(t *testing.T) {
	orgID := uuid.New()
	k, _, err := apikey.NewScheme("").MintOrg(orgID, "x", nil, apikey.ProjectGrant{All: true}, future(), time.Now().UTC())
	require.NoError(t, err)
	refs := fakes.NewPublicIDs()
	orgPub := refs.Org(orgID)
	svc := apikey.NewService(&fakes.APIKeyStore{Keys: []*apikey.APIKey{k}}, apikey.NewScheme(""), stubProjects{ids: []uuid.UUID{knownProject}}, nil)
	cp := apikey.NewControlPlane(svc, refs)
	list := func(p session.Principal) error {
		_, err := cp.ListAPIKeys(session.PrincipalInto(context.Background(), p), connect.NewRequest(&apikeyv1.ListAPIKeysRequest{OrgId: orgPub}))
		return err
	}

	err = list(session.Principal{Source: session.SourceAPIKey, ActiveOrgID: orgID, Scopes: []string{"apikey:read"}, ProjectIDs: []uuid.UUID{knownProject}})
	assert.True(t, apikey.IsNotFoundError(err), "%v", err)
	err = list(session.Principal{Source: session.SourceAPIKey, ActiveOrgID: orgID, Scopes: []string{"apikey:read"}, AllProjects: true, ResourceIDs: []uuid.UUID{uuid.New()}})
	assert.True(t, apikey.IsNotFoundError(err), "a key limited to some resources is refused too: %v", err)
	require.NoError(t, list(session.Principal{Source: session.SourceAPIKey, ActiveOrgID: orgID, Scopes: []string{"apikey:read"}, AllProjects: true}))
}
