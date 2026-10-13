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
	refs := fakes.NewPublicIDs()
	orgA, orgB := uuid.New(), uuid.New()
	refs.Org(orgA)
	orgBPub := refs.Org(orgB)
	foreign := mustProject(t, orgB)
	store := &fakes.ProjectStore{Project: foreign}
	cp := project.NewControlPlane(project.NewService(store, nil), refs)
	for name, p := range map[string]session.Principal{
		"member of A": {Source: session.SourceLocal, UserID: uuid.New(), ActiveOrgID: orgA, Scopes: []string{"*"}},
		"key of A":    {Source: session.SourceAPIKey, ActiveOrgID: orgA, AllProjects: true},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := session.PrincipalInto(context.Background(), p)
			for _, id := range []string{foreign.PublicID, foreign.ID.String()} {
				_, err := cp.GetProject(ctx, connect.NewRequest(&projectv1.GetProjectRequest{Id: id}))
				assert.True(t, project.IsNotFoundError(err), "GetProject %s: %v", id, err)
				_, err = cp.UpdateProject(ctx, connect.NewRequest(&projectv1.UpdateProjectRequest{Id: id, Name: "Hacked"}))
				assert.True(t, project.IsNotFoundError(err), "UpdateProject %s: %v", id, err)
			}
			assert.Nil(t, store.Saved, "nothing written")
			for _, orgID := range []string{orgBPub, orgB.String()} {
				_, err := cp.CreateProject(ctx, connect.NewRequest(&projectv1.CreateProjectRequest{OrgId: orgID, Slug: "x", Name: "X"}))
				assert.True(t, project.IsNotFoundError(err), "CreateProject %s: %v", orgID, err)
			}
			assert.Nil(t, store.Saved, "nothing written")
		})
	}
}

func TestProjectReach_OrgKeyCannotListAnotherOrg(t *testing.T) {
	refs := fakes.NewPublicIDs()
	orgA, orgB := uuid.New(), uuid.New()
	refs.Org(orgA)
	orgBPub := refs.Org(orgB)
	cp := project.NewControlPlane(project.NewService(&fakes.ProjectStore{Projects: []*project.Project{mustProject(t, orgB)}}, nil), refs)
	ctx := session.PrincipalInto(context.Background(), session.Principal{Source: session.SourceAPIKey, ActiveOrgID: orgA, AllProjects: true})
	for _, orgID := range []string{orgBPub, orgB.String(), orgA.String()} {
		_, err := cp.ListProjects(ctx, connect.NewRequest(&projectv1.ListProjectsRequest{OrgId: orgID}))
		assert.True(t, project.IsNotFoundError(err), "ListProjects %s: %v", orgID, err)
	}
}

func TestProjectReach_ListShowsOnlyGrantedProjects(t *testing.T) {
	refs := fakes.NewPublicIDs()
	orgA := uuid.New()
	orgAPub := refs.Org(orgA)
	granted, other := mustProject(t, orgA), mustProject(t, orgA)
	cp := project.NewControlPlane(project.NewService(&fakes.ProjectStore{Projects: []*project.Project{granted, other}}, nil), refs)
	ctx := session.PrincipalInto(context.Background(), session.Principal{Source: session.SourceAPIKey, ActiveOrgID: orgA, ProjectIDs: []uuid.UUID{granted.ID}})
	resp, err := cp.ListProjects(ctx, connect.NewRequest(&projectv1.ListProjectsRequest{OrgId: orgAPub}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.GetProjects(), 1)
	assert.Equal(t, granted.PublicID, resp.Msg.GetProjects()[0].GetId())

	_, err = cp.CreateProject(ctx, connect.NewRequest(&projectv1.CreateProjectRequest{OrgId: orgAPub, Slug: "x", Name: "X"}))
	assert.True(t, project.IsNotFoundError(err), "a key granted some projects cannot create one")
}

func TestProjectReach_ProjectKeyCannotReachAnotherProject(t *testing.T) {
	refs := fakes.NewPublicIDs()
	orgA := uuid.New()
	refs.Org(orgA)
	mine, theirs := mustProject(t, orgA), mustProject(t, orgA)
	ctx := session.PrincipalInto(context.Background(), session.Principal{Source: session.SourceAPIKey, ActiveOrgID: orgA, ActiveProjectID: mine.ID, ProjectIDs: []uuid.UUID{mine.ID}})

	_, err := project.NewControlPlane(project.NewService(&fakes.ProjectStore{Project: theirs}, nil), refs).
		GetProject(ctx, connect.NewRequest(&projectv1.GetProjectRequest{Id: theirs.PublicID}))
	assert.True(t, project.IsNotFoundError(err))

	resp, err := project.NewControlPlane(project.NewService(&fakes.ProjectStore{Project: mine}, nil), refs).
		GetProject(ctx, connect.NewRequest(&projectv1.GetProjectRequest{Id: mine.PublicID}))
	require.NoError(t, err)
	assert.Equal(t, mine.PublicID, resp.Msg.GetProject().GetId())
}
