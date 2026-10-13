package project_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/project"
	"altalune.id/oagent/internal/testutil/fakes"
)

func testUnexpected() apperror.UnexpectedFunc {
	return apperror.NewReporter(nil, false).Unexpected
}

func TestService_Create(t *testing.T) {
	store := &fakes.ProjectStore{}
	svc := project.NewService(store, testUnexpected())

	orgID := uuid.New()
	p, err := svc.Create(context.Background(), project.CreateRequest{OrgID: orgID, Slug: "acme", Name: "Acme", CreatedBy: uuid.New()})
	require.NoError(t, err)
	assert.Equal(t, p, store.Saved)
	assert.Equal(t, orgID, p.OrgID)
	assert.Equal(t, "acme", p.Slug)
}

func TestService_Create_MissingOrg(t *testing.T) {
	_, err := project.NewService(&fakes.ProjectStore{}, testUnexpected()).Create(context.Background(), project.CreateRequest{Name: "Acme"})
	_, ok := apperror.AsAppError(err)
	assert.True(t, ok)
}

func TestService_Create_GeneratedSlug(t *testing.T) {
	svc := project.NewService(&fakes.ProjectStore{}, testUnexpected())

	p, err := svc.Create(context.Background(), project.CreateRequest{OrgID: uuid.New(), Name: "Acme"})
	require.NoError(t, err)
	assert.NotEmpty(t, p.Slug)
	assert.NoError(t, project.ValidateSlug(p.Slug))
}

func TestService_Create_GeneratedSlugRetries(t *testing.T) {
	calls := 0
	store := &fakes.ProjectStore{SaveFn: func(*project.Project) error {
		calls++
		if calls < 3 {
			return &project.AlreadyExistsError{}
		}
		return nil
	}}

	p, err := project.NewService(store, testUnexpected()).Create(context.Background(), project.CreateRequest{OrgID: uuid.New(), Name: "Acme"})
	require.NoError(t, err)
	assert.Equal(t, 3, calls)
	assert.NotEmpty(t, p.Slug)
}

func TestService_Create_GeneratedSlugExhausted(t *testing.T) {
	store := &fakes.ProjectStore{SaveFn: func(*project.Project) error { return &project.AlreadyExistsError{} }}

	_, err := project.NewService(store, testUnexpected()).Create(context.Background(), project.CreateRequest{OrgID: uuid.New(), Name: "Acme"})
	assert.True(t, project.IsAlreadyExistsError(err))
}

func TestService_Create_Validation(t *testing.T) {
	store := &fakes.ProjectStore{}
	svc := project.NewService(store, testUnexpected())

	_, err := svc.Create(context.Background(), project.CreateRequest{OrgID: uuid.New(), Slug: "ab", Name: "Acme"})
	assert.True(t, project.IsInvalidSlugError(err))
	assert.Nil(t, store.Saved)
}

func TestService_Create_StoreError(t *testing.T) {
	store := &fakes.ProjectStore{Err: &project.AlreadyExistsError{Slug: "acme"}}
	svc := project.NewService(store, testUnexpected())

	_, err := svc.Create(context.Background(), project.CreateRequest{OrgID: uuid.New(), Slug: "acme", Name: "Acme"})
	assert.True(t, project.IsAlreadyExistsError(err))
}

func TestService_GetAndList(t *testing.T) {
	p, err := project.NewProject(uuid.New(), "acme", "Acme", uuid.New())
	require.NoError(t, err)
	store := &fakes.ProjectStore{Project: p, Projects: []*project.Project{p}}
	svc := project.NewService(store, testUnexpected())

	got, err := svc.Get(context.Background(), p.ID)
	require.NoError(t, err)
	assert.Equal(t, p, got)

	bySlug, err := svc.BySlug(context.Background(), p.OrgID, "acme")
	require.NoError(t, err)
	assert.Equal(t, p, bySlug)

	all, err := svc.List(context.Background(), p.OrgID)
	require.NoError(t, err)
	assert.Equal(t, []*project.Project{p}, all)
}

func TestService_Get_StoreError(t *testing.T) {
	svc := project.NewService(&fakes.ProjectStore{Err: errors.New("boom")}, testUnexpected())

	_, err := svc.Get(context.Background(), uuid.New())
	assert.Error(t, err)
	_, err = svc.BySlug(context.Background(), uuid.New(), "acme")
	assert.Error(t, err)
	_, err = svc.List(context.Background(), uuid.New())
	assert.Error(t, err)
}

func TestService_Unexpected(t *testing.T) {
	svc := project.NewService(&fakes.ProjectStore{Err: errors.New("boom")}, testUnexpected())

	_, err := svc.Get(context.Background(), uuid.New())
	_, ok := apperror.AsAppError(err)
	assert.True(t, ok)
}

func TestService_Update(t *testing.T) {
	p, err := project.NewProject(uuid.New(), "acme", "Acme", uuid.New())
	require.NoError(t, err)
	store := &fakes.ProjectStore{Project: p}

	got, err := project.NewService(store, testUnexpected()).Update(context.Background(), p.ID, "Acme Two", "acme-two")
	require.NoError(t, err)
	assert.Equal(t, "Acme Two", got.Name)
	assert.Equal(t, "acme-two", got.Slug)
	assert.Equal(t, got, store.Saved)
}

func TestService_Update_NotFound(t *testing.T) {
	store := &fakes.ProjectStore{Err: &project.NotFoundError{ID: "x"}}

	_, err := project.NewService(store, testUnexpected()).Update(context.Background(), uuid.New(), "Acme Two", "acme-two")
	assert.Error(t, err)
}

func TestService_Update_InvalidName(t *testing.T) {
	p, err := project.NewProject(uuid.New(), "acme", "Acme", uuid.New())
	require.NoError(t, err)

	_, err = project.NewService(&fakes.ProjectStore{Project: p}, testUnexpected()).Update(context.Background(), p.ID, "", "acme-two")
	assert.True(t, project.IsInvalidNameError(err))
}

func TestService_Update_InvalidSlug(t *testing.T) {
	p, err := project.NewProject(uuid.New(), "acme", "Acme", uuid.New())
	require.NoError(t, err)

	_, err = project.NewService(&fakes.ProjectStore{Project: p}, testUnexpected()).Update(context.Background(), p.ID, "Acme", "Bad_Slug")
	assert.True(t, project.IsInvalidSlugError(err))
}

func TestService_ByPublicID(t *testing.T) {
	p, err := project.NewProject(uuid.New(), "web", "Web", uuid.Nil)
	require.NoError(t, err)
	got, err := project.NewService(&fakes.ProjectStore{Project: p}, testUnexpected()).ByPublicID(context.Background(), p.PublicID)
	require.NoError(t, err)
	assert.Same(t, p, got)

	_, err = project.NewService(&fakes.ProjectStore{Project: p}, testUnexpected()).ByPublicID(context.Background(), "nope")
	assert.True(t, project.IsNotFoundError(err))
	_, err = project.NewService(&fakes.ProjectStore{Err: errors.New("down")}, testUnexpected()).ByPublicID(context.Background(), p.PublicID)
	_, isApp := apperror.AsAppError(err)
	assert.True(t, isApp)
}
