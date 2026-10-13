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

func newProjectServiceClient(t *testing.T, store project.Store, refs project.OrgRefs) projectv1connect.ProjectServiceClient {
	t.Helper()
	_, handler := projectv1connect.NewProjectServiceHandler(project.NewControlPlane(project.NewService(store, nil), refs),
		connect.WithInterceptors(fakes.OperatorInterceptor()))
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	return projectv1connect.NewProjectServiceClient(http.DefaultClient, ts.URL)
}

func TestProjectService_CreateProject(t *testing.T) {
	refs := fakes.NewPublicIDs()
	orgPub := refs.Org(uuid.New())
	store := &fakes.ProjectStore{}
	c := newProjectServiceClient(t, store, refs)

	resp, err := c.CreateProject(context.Background(), connect.NewRequest(&projectv1.CreateProjectRequest{OrgId: orgPub, Slug: "acme", Name: "Acme"}))
	require.NoError(t, err)
	require.NotNil(t, store.Saved)
	assert.Equal(t, "acme", resp.Msg.GetProject().GetSlug())
	assert.Equal(t, "Acme", resp.Msg.GetProject().GetName())
	assert.Equal(t, store.Saved.PublicID, resp.Msg.GetProject().GetId())
	assert.Equal(t, orgPub, resp.Msg.GetProject().GetOrgId())
}

func TestProjectService_CreateProject_UnknownOrgID(t *testing.T) {
	refs := fakes.NewPublicIDs()
	orgID := uuid.New()
	refs.Org(orgID)
	c := newProjectServiceClient(t, &fakes.ProjectStore{}, refs)

	for _, id := range []string{"nope", orgID.String()} {
		_, err := c.CreateProject(context.Background(), connect.NewRequest(&projectv1.CreateProjectRequest{OrgId: id, Name: "Acme"}))
		assert.Error(t, err, id)
	}
}

func TestProjectService_GetProject(t *testing.T) {
	refs := fakes.NewPublicIDs()
	orgID := uuid.New()
	orgPub := refs.Org(orgID)
	p, err := project.NewProject(orgID, "acme", "Acme", uuid.New())
	require.NoError(t, err)
	c := newProjectServiceClient(t, &fakes.ProjectStore{Project: p}, refs)

	resp, err := c.GetProject(context.Background(), connect.NewRequest(&projectv1.GetProjectRequest{Id: p.PublicID}))
	require.NoError(t, err)
	assert.Equal(t, "acme", resp.Msg.GetProject().GetSlug())
	assert.Equal(t, p.PublicID, resp.Msg.GetProject().GetId())
	assert.Equal(t, orgPub, resp.Msg.GetProject().GetOrgId())
}

func TestProjectService_GetProject_UnknownID(t *testing.T) {
	p, err := project.NewProject(uuid.New(), "acme", "Acme", uuid.New())
	require.NoError(t, err)
	c := newProjectServiceClient(t, &fakes.ProjectStore{Project: p}, fakes.NewPublicIDs())

	for _, id := range []string{"not-a-public-id", p.ID.String()} {
		_, err := c.GetProject(context.Background(), connect.NewRequest(&projectv1.GetProjectRequest{Id: id}))
		assert.Error(t, err, id)
	}
}

func TestProjectService_ListProjects(t *testing.T) {
	refs := fakes.NewPublicIDs()
	orgID := uuid.New()
	orgPub := refs.Org(orgID)
	p1, err := project.NewProject(orgID, "aaa", "A", uuid.New())
	require.NoError(t, err)
	p2, err := project.NewProject(orgID, "bbb", "B", uuid.New())
	require.NoError(t, err)
	c := newProjectServiceClient(t, &fakes.ProjectStore{Projects: []*project.Project{p1, p2}}, refs)

	resp, err := c.ListProjects(context.Background(), connect.NewRequest(&projectv1.ListProjectsRequest{OrgId: orgPub}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.GetProjects(), 2)
	assert.Equal(t, p1.PublicID, resp.Msg.GetProjects()[0].GetId())
	assert.Equal(t, orgPub, resp.Msg.GetProjects()[0].GetOrgId())
}

func TestProjectService_ListProjects_UnknownOrgID(t *testing.T) {
	c := newProjectServiceClient(t, &fakes.ProjectStore{}, fakes.NewPublicIDs())

	_, err := c.ListProjects(context.Background(), connect.NewRequest(&projectv1.ListProjectsRequest{OrgId: "not-a-public-id"}))
	assert.Error(t, err)
}

func TestProjectService_ListProjects_StoreError(t *testing.T) {
	refs := fakes.NewPublicIDs()
	c := newProjectServiceClient(t, &fakes.ProjectStore{Err: &project.NotFoundError{Slug: "acme"}}, refs)

	_, err := c.ListProjects(context.Background(), connect.NewRequest(&projectv1.ListProjectsRequest{OrgId: refs.Org(uuid.New())}))
	assert.Error(t, err)
}
