package project_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	projectv1 "altalune.id/oagent/gen/go/project/v1"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/project"
	"altalune.id/oagent/internal/testutil/fakes"
)

func mustProject(t *testing.T, orgID uuid.UUID) *project.Project {
	t.Helper()
	p, err := project.NewProject(orgID, "prj-"+uuid.NewString()[:6], "Project", uuid.Nil)
	require.NoError(t, err)
	return p
}

func TestProjectReach_IDOnlyRPCs(t *testing.T) {
	orgA, orgB := uuid.New(), uuid.New()
	foreign := mustProject(t, orgB)
	store := &fakes.ProjectStore{Project: foreign}
	cp := project.NewControlPlane(project.NewService(store, nil))
	for name, p := range map[string]session.Principal{
		"member of A": {Source: session.SourceLocal, UserID: uuid.New(), ActiveOrgID: orgA, Scopes: []string{"*"}},
		"key of A":    {Source: session.SourceAPIKey, ActiveOrgID: orgA, AllProjects: true},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := session.PrincipalInto(context.Background(), p)
			_, err := cp.GetProject(ctx, connect.NewRequest(&projectv1.GetProjectRequest{Id: foreign.ID.String()}))
			assert.True(t, project.IsNotFoundError(err), "GetProject: %v", err)
			_, err = cp.UpdateProject(ctx, connect.NewRequest(&projectv1.UpdateProjectRequest{Id: foreign.ID.String(), Name: "Hacked"}))
			assert.True(t, project.IsNotFoundError(err), "UpdateProject: %v", err)
			assert.Nil(t, store.Saved, "nothing written")
			_, err = cp.CreateProject(ctx, connect.NewRequest(&projectv1.CreateProjectRequest{OrgId: orgB.String(), Slug: "x", Name: "X"}))
			assert.True(t, project.IsNotFoundError(err), "CreateProject: %v", err)
		})
	}
}

func TestProjectReach_OrgKeyCannotListAnotherOrg(t *testing.T) {
	orgA, orgB := uuid.New(), uuid.New()
	cp := project.NewControlPlane(project.NewService(&fakes.ProjectStore{Projects: []*project.Project{mustProject(t, orgB)}}, nil))
	ctx := session.PrincipalInto(context.Background(), session.Principal{Source: session.SourceAPIKey, ActiveOrgID: orgA, AllProjects: true})
	_, err := cp.ListProjects(ctx, connect.NewRequest(&projectv1.ListProjectsRequest{OrgId: orgB.String()}))
	assert.True(t, project.IsNotFoundError(err))
}

func TestProjectReach_ListShowsOnlyGrantedProjects(t *testing.T) {
	orgA := uuid.New()
	granted, other := mustProject(t, orgA), mustProject(t, orgA)
	cp := project.NewControlPlane(project.NewService(&fakes.ProjectStore{Projects: []*project.Project{granted, other}}, nil))
	ctx := session.PrincipalInto(context.Background(), session.Principal{Source: session.SourceAPIKey, ActiveOrgID: orgA, ProjectIDs: []uuid.UUID{granted.ID}})
	resp, err := cp.ListProjects(ctx, connect.NewRequest(&projectv1.ListProjectsRequest{OrgId: orgA.String()}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.GetProjects(), 1)
	assert.Equal(t, granted.ID.String(), resp.Msg.GetProjects()[0].GetId())

	_, err = cp.CreateProject(ctx, connect.NewRequest(&projectv1.CreateProjectRequest{OrgId: orgA.String(), Slug: "x", Name: "X"}))
	assert.True(t, project.IsNotFoundError(err), "a key granted some projects cannot create one")
}

func TestProjectReach_ProjectKeyCannotReachAnotherProject(t *testing.T) {
	orgA := uuid.New()
	mine, theirs := mustProject(t, orgA), mustProject(t, orgA)
	ctx := session.PrincipalInto(context.Background(), session.Principal{Source: session.SourceAPIKey, ActiveOrgID: orgA, ActiveProjectID: mine.ID, ProjectIDs: []uuid.UUID{mine.ID}})

	_, err := project.NewControlPlane(project.NewService(&fakes.ProjectStore{Project: theirs}, nil)).
		GetProject(ctx, connect.NewRequest(&projectv1.GetProjectRequest{Id: theirs.ID.String()}))
	assert.True(t, project.IsNotFoundError(err))

	_, err = project.NewControlPlane(project.NewService(&fakes.ProjectStore{Project: mine}, nil)).
		GetProject(ctx, connect.NewRequest(&projectv1.GetProjectRequest{Id: mine.ID.String()}))
	require.NoError(t, err)
}
