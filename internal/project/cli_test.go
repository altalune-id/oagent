package project_test

import (
	"bytes"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	projectv1connect "altalune.id/oagent/gen/go/project/v1/projectv1connect"
	"altalune.id/oagent/internal/apiclient"
	"altalune.id/oagent/internal/controlplane"
	"altalune.id/oagent/internal/project"
	"altalune.id/oagent/internal/testutil/cptest"
	"altalune.id/oagent/internal/testutil/fakes"
)

func projectClient(t *testing.T, store project.Store, refs *fakes.PublicIDs) projectv1connect.ProjectServiceClient {
	t.Helper()
	srv := controlplane.New(controlplane.Services{Projects: project.NewService(store, nil), Refs: refs}, nil, cptest.OperatorOptions())
	ts := httptest.NewServer(srv.Handler(""))
	t.Cleanup(ts.Close)
	return apiclient.New(ts.URL, cptest.OperatorCredential()).Projects
}

func runProject(t *testing.T, client projectv1connect.ProjectServiceClient, args ...string) (*bytes.Buffer, error) {
	t.Helper()
	buf := new(bytes.Buffer)
	root := &cobra.Command{Use: "x", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().String("output", "", "")
	root.AddGroup(&cobra.Group{ID: "domain", Title: "Domain:"})
	root.AddCommand(project.NewCmd(func(*cobra.Command) (projectv1connect.ProjectServiceClient, error) { return client, nil }))
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(append([]string{"project"}, args...))
	return buf, root.ExecuteContext(t.Context())
}

func TestProjectCreate(t *testing.T) {
	refs := fakes.NewPublicIDs()
	store := &fakes.ProjectStore{}
	buf, err := runProject(t, projectClient(t, store, refs), "create", "--org-id", refs.Org(uuid.New()), "--name", "Acme", "--slug", "acme", "--output", "text")

	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Created project acme ("+store.Saved.PublicID+")")
}

func TestProjectCreate_JSON(t *testing.T) {
	refs := fakes.NewPublicIDs()
	orgPub := refs.Org(uuid.New())
	buf, err := runProject(t, projectClient(t, &fakes.ProjectStore{}, refs), "create", "--org-id", orgPub, "--name", "Acme", "--output", "json")

	require.NoError(t, err)
	assert.Contains(t, buf.String(), `"name": "Acme"`)
	assert.Contains(t, buf.String(), `"orgId": "`+orgPub+`"`)
}

func TestProjectGet_Text(t *testing.T) {
	refs := fakes.NewPublicIDs()
	p, err := project.NewProject(uuid.New(), "acme", "Acme", uuid.New())
	require.NoError(t, err)
	orgPub := refs.Org(p.OrgID)
	store := &fakes.ProjectStore{Project: p}

	buf, err := runProject(t, projectClient(t, store, refs), "get", p.PublicID, "--output", "text")

	require.NoError(t, err)
	assert.Contains(t, buf.String(), "SLUG")
	assert.Contains(t, buf.String(), "acme")
	assert.Contains(t, buf.String(), p.PublicID)
	assert.Contains(t, buf.String(), orgPub)
	assert.NotContains(t, buf.String(), p.ID.String())
}

func TestProjectList_JSON(t *testing.T) {
	p, err := project.NewProject(uuid.New(), "acme", "Acme", uuid.New())
	require.NoError(t, err)
	refs := fakes.NewPublicIDs()
	store := &fakes.ProjectStore{Projects: []*project.Project{p}}

	buf, err := runProject(t, projectClient(t, store, refs), "list", "--org-id", refs.Org(p.OrgID), "--output", "json")

	require.NoError(t, err)
	assert.Contains(t, buf.String(), `"slug": "acme"`)
	assert.Contains(t, buf.String(), `"id": "`+p.PublicID+`"`)
}

func TestProjectGet_InvalidID(t *testing.T) {
	_, err := runProject(t, projectClient(t, &fakes.ProjectStore{}, fakes.NewPublicIDs()), "get", "not-a-public-id")
	assert.Error(t, err)
}
