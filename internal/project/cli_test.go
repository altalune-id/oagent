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
	"altalune.id/oagent/internal/controlplane"
	"altalune.id/oagent/internal/project"
	"altalune.id/oagent/internal/testutil/fakes"
)

func projectClient(t *testing.T, store project.Store) projectv1connect.ProjectServiceClient {
	t.Helper()
	srv := controlplane.New(controlplane.Services{Projects: project.NewService(store, nil)}, nil, controlplane.Options{})
	ts := httptest.NewServer(srv.Handler(""))
	t.Cleanup(ts.Close)
	return controlplane.NewClient(ts.URL, "").Projects
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
	buf, err := runProject(t, projectClient(t, &fakes.ProjectStore{}), "create", "--org-id", uuid.New().String(), "--name", "Acme", "--slug", "acme", "--output", "text")

	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Created project acme")
}

func TestProjectCreate_JSON(t *testing.T) {
	buf, err := runProject(t, projectClient(t, &fakes.ProjectStore{}), "create", "--org-id", uuid.New().String(), "--name", "Acme", "--output", "json")

	require.NoError(t, err)
	assert.Contains(t, buf.String(), `"name": "Acme"`)
}

func TestProjectGet_Text(t *testing.T) {
	p, err := project.NewProject(uuid.New(), "acme", "Acme", uuid.New())
	require.NoError(t, err)
	store := &fakes.ProjectStore{Project: p}

	buf, err := runProject(t, projectClient(t, store), "get", p.ID.String(), "--output", "text")

	require.NoError(t, err)
	assert.Contains(t, buf.String(), "SLUG")
	assert.Contains(t, buf.String(), "acme")
}

func TestProjectList_JSON(t *testing.T) {
	p, err := project.NewProject(uuid.New(), "acme", "Acme", uuid.New())
	require.NoError(t, err)
	store := &fakes.ProjectStore{Projects: []*project.Project{p}}

	buf, err := runProject(t, projectClient(t, store), "list", "--org-id", p.OrgID.String(), "--output", "json")

	require.NoError(t, err)
	assert.Contains(t, buf.String(), `"slug": "acme"`)
}

func TestProjectGet_InvalidID(t *testing.T) {
	_, err := runProject(t, projectClient(t, &fakes.ProjectStore{}), "get", "not-a-uuid")
	assert.Error(t, err)
}
