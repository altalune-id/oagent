package project_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	projectv1 "altalune.id/oagent/gen/go/project/v1"
	projectv1connect "altalune.id/oagent/gen/go/project/v1/projectv1connect"
	"altalune.id/oagent/internal/project"
	"altalune.id/oagent/internal/testutil/fakes"
)

func newProjectServiceClient(t *testing.T, store project.Store) projectv1connect.ProjectServiceClient {
	t.Helper()
	_, handler := projectv1connect.NewProjectServiceHandler(project.NewControlPlane(project.NewService(store, nil)),
		connect.WithInterceptors(fakes.OperatorInterceptor()))
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	return projectv1connect.NewProjectServiceClient(http.DefaultClient, ts.URL)
}

func TestProjectService_CreateProject(t *testing.T) {
	c := newProjectServiceClient(t, &fakes.ProjectStore{})

	resp, err := c.CreateProject(context.Background(), connect.NewRequest(&projectv1.CreateProjectRequest{OrgId: uuid.New().String(), Slug: "acme", Name: "Acme"}))
	require.NoError(t, err)
	assert.Equal(t, "acme", resp.Msg.GetProject().GetSlug())
	assert.Equal(t, "Acme", resp.Msg.GetProject().GetName())
	assert.NotEmpty(t, resp.Msg.GetProject().GetPublicId())
	assert.NotEmpty(t, resp.Msg.GetProject().GetOrgId())
}

func TestProjectService_CreateProject_InvalidOrgID(t *testing.T) {
	c := newProjectServiceClient(t, &fakes.ProjectStore{})

	_, err := c.CreateProject(context.Background(), connect.NewRequest(&projectv1.CreateProjectRequest{OrgId: "nope", Name: "Acme"}))
	assert.Error(t, err)
}

func TestProjectService_GetProject(t *testing.T) {
	p, err := project.NewProject(uuid.New(), "acme", "Acme", uuid.New())
	require.NoError(t, err)
	c := newProjectServiceClient(t, &fakes.ProjectStore{Project: p})

	resp, err := c.GetProject(context.Background(), connect.NewRequest(&projectv1.GetProjectRequest{Id: p.ID.String()}))
	require.NoError(t, err)
	assert.Equal(t, "acme", resp.Msg.GetProject().GetSlug())
}

func TestProjectService_GetProject_InvalidID(t *testing.T) {
	c := newProjectServiceClient(t, &fakes.ProjectStore{})

	_, err := c.GetProject(context.Background(), connect.NewRequest(&projectv1.GetProjectRequest{Id: "not-a-uuid"}))
	assert.Error(t, err)
}

func TestProjectService_ListProjects(t *testing.T) {
	p1, err := project.NewProject(uuid.New(), "aaa", "A", uuid.New())
	require.NoError(t, err)
	p2, err := project.NewProject(uuid.New(), "bbb", "B", uuid.New())
	require.NoError(t, err)
	c := newProjectServiceClient(t, &fakes.ProjectStore{Projects: []*project.Project{p1, p2}})

	resp, err := c.ListProjects(context.Background(), connect.NewRequest(&projectv1.ListProjectsRequest{OrgId: uuid.New().String()}))
	require.NoError(t, err)
	assert.Len(t, resp.Msg.GetProjects(), 2)
}

func TestProjectService_ListProjects_InvalidOrgID(t *testing.T) {
	c := newProjectServiceClient(t, &fakes.ProjectStore{})

	_, err := c.ListProjects(context.Background(), connect.NewRequest(&projectv1.ListProjectsRequest{OrgId: "not-a-uuid"}))
	assert.Error(t, err)
}

func TestProjectService_ListProjects_StoreError(t *testing.T) {
	c := newProjectServiceClient(t, &fakes.ProjectStore{Err: &project.NotFoundError{Slug: "acme"}})

	_, err := c.ListProjects(context.Background(), connect.NewRequest(&projectv1.ListProjectsRequest{OrgId: uuid.New().String()}))
	assert.Error(t, err)
}
