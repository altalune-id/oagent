package apikey_test

import (
	"bytes"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apikeyv1connect "altalune.id/oagent/gen/go/apikey/v1/apikeyv1connect"
	"altalune.id/oagent/internal/apiclient"
	"altalune.id/oagent/internal/apikey"
	"altalune.id/oagent/internal/controlplane"
	"altalune.id/oagent/internal/testutil/fakes"
)

func apiKeyClient(t *testing.T, store apikey.Store) apikeyv1connect.APIKeyServiceClient {
	t.Helper()
	svc := apikey.NewService(store, apikey.NewScheme(""), stubProjects{ids: []uuid.UUID{knownProject}}, nil)
	srv := controlplane.New(controlplane.Services{APIKeys: svc}, nil, controlplane.Options{})
	ts := httptest.NewServer(srv.Handler(""))
	t.Cleanup(ts.Close)
	return apiclient.New(ts.URL, apiclient.Credential{}).APIKeys
}

func runAPIKey(t *testing.T, client apikeyv1connect.APIKeyServiceClient, args ...string) (*bytes.Buffer, error) {
	t.Helper()
	buf := new(bytes.Buffer)
	root := &cobra.Command{Use: "x", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().String("output", "", "")
	root.AddGroup(&cobra.Group{ID: "auth", Title: "Auth:"})
	root.AddCommand(apikey.NewCmd(func(*cobra.Command) (apikeyv1connect.APIKeyServiceClient, error) { return client, nil }))
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(append([]string{"apikey"}, args...))
	return buf, root.ExecuteContext(t.Context())
}

func TestAPIKeyCreate(t *testing.T) {
	buf, err := runAPIKey(t, apiKeyClient(t, &fakes.APIKeyStore{}), "create",
		"--org-id", uuid.New().String(), "--name", "deploy", "--all-projects", "--output", "text")

	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Created API key deploy")
	assert.Contains(t, buf.String(), "Secret:")
}

func TestAPIKeyCreate_JSON(t *testing.T) {
	buf, err := runAPIKey(t, apiKeyClient(t, &fakes.APIKeyStore{}), "create",
		"--org-id", uuid.New().String(), "--name", "deploy", "--all-projects", "--output", "json")

	require.NoError(t, err)
	assert.Contains(t, buf.String(), `"plaintext"`)
}

func TestAPIKeyList_JSON(t *testing.T) {
	k, _, err := apikey.NewScheme("").MintOrg(uuid.New(), "deploy", nil, apikey.ProjectGrant{All: true}, future(), time.Now().UTC())
	require.NoError(t, err)
	store := &fakes.APIKeyStore{Keys: []*apikey.APIKey{k}}

	buf, err := runAPIKey(t, apiKeyClient(t, store), "list", "--org-id", k.OrgID.String(), "--output", "json")

	require.NoError(t, err)
	assert.Contains(t, buf.String(), `"name": "deploy"`)
}

func TestAPIKeyRevoke(t *testing.T) {
	orgID := uuid.New()
	k, _, err := apikey.NewScheme("").MintOrg(orgID, "deploy", nil, apikey.ProjectGrant{All: true}, future(), time.Now().UTC())
	require.NoError(t, err)

	buf, err := runAPIKey(t, apiKeyClient(t, &fakes.APIKeyStore{Key: k}), "revoke", k.ID.String(), "--org-id", orgID.String())

	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Revoked API key")
}

func TestAPIKeyCreate_ProjectKeyScopes(t *testing.T) {
	buf, err := runAPIKey(t, apiKeyClient(t, &fakes.APIKeyStore{}), "create",
		"--org-id", uuid.New().String(), "--kind", "project", "--project-id", knownProject.String(),
		"--name", "ci", "--scopes", "agent:read,agent:write", "--output", "text")

	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Created API key ci")
}

func TestAPIKeyList_Text(t *testing.T) {
	k, _, err := apikey.NewScheme("").MintOrg(uuid.New(), "deploy", nil, apikey.ProjectGrant{All: true}, future(), time.Now().UTC())
	require.NoError(t, err)
	store := &fakes.APIKeyStore{Keys: []*apikey.APIKey{k}}

	buf, err := runAPIKey(t, apiKeyClient(t, store), "list", "--org-id", k.OrgID.String(), "--output", "text")

	require.NoError(t, err)
	assert.Contains(t, buf.String(), "KIND")
}
