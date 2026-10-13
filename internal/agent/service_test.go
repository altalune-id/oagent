package agent_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/agent"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/testutil/fakes"
)

func testUnexpected() apperror.UnexpectedFunc {
	return apperror.NewReporter(nil, false).Unexpected
}

func TestService_Create(t *testing.T) {
	store := &fakes.AgentStore{}
	svc := agent.NewService(store, fakes.Transactor{}, testUnexpected())

	projectID := uuid.New()
	a, err := svc.Create(context.Background(), agent.CreateRequest{OrgID: uuid.New(), ProjectID: projectID, Slug: "acme", Name: "Acme", Description: "d"})
	require.NoError(t, err)
	assert.Equal(t, a, store.Saved)
	assert.Equal(t, projectID, a.ProjectID)
}

func TestService_Create_MissingScope(t *testing.T) {
	svc := agent.NewService(&fakes.AgentStore{}, fakes.Transactor{}, testUnexpected())

	_, err := svc.Create(context.Background(), agent.CreateRequest{ProjectID: uuid.New(), Name: "Acme"})
	_, ok := apperror.AsAppError(err)
	assert.True(t, ok)

	_, err = svc.Create(context.Background(), agent.CreateRequest{OrgID: uuid.New(), Name: "Acme"})
	_, ok = apperror.AsAppError(err)
	assert.True(t, ok)
}

func TestService_Create_GeneratedSlug(t *testing.T) {
	svc := agent.NewService(&fakes.AgentStore{}, fakes.Transactor{}, testUnexpected())

	a, err := svc.Create(context.Background(), agent.CreateRequest{OrgID: uuid.New(), ProjectID: uuid.New(), Name: "Acme"})
	require.NoError(t, err)
	assert.NotEmpty(t, a.Slug)
	assert.NoError(t, agent.ValidateSlug(a.Slug))
}

func TestService_Create_GeneratedSlugRetries(t *testing.T) {
	calls := 0
	store := &fakes.AgentStore{SaveFn: func(*agent.Agent) error {
		calls++
		if calls < 3 {
			return &agent.AlreadyExistsError{}
		}
		return nil
	}}

	a, err := agent.NewService(store, fakes.Transactor{}, testUnexpected()).Create(context.Background(), agent.CreateRequest{OrgID: uuid.New(), ProjectID: uuid.New(), Name: "Acme"})
	require.NoError(t, err)
	assert.Equal(t, 3, calls)
	assert.NotEmpty(t, a.Slug)
}

func TestService_Create_GeneratedSlugExhausted(t *testing.T) {
	store := &fakes.AgentStore{SaveFn: func(*agent.Agent) error { return &agent.AlreadyExistsError{} }}

	_, err := agent.NewService(store, fakes.Transactor{}, testUnexpected()).Create(context.Background(), agent.CreateRequest{OrgID: uuid.New(), ProjectID: uuid.New(), Name: "Acme"})
	assert.True(t, agent.IsAlreadyExistsError(err))
}

func TestService_Create_Validation(t *testing.T) {
	store := &fakes.AgentStore{}
	svc := agent.NewService(store, fakes.Transactor{}, testUnexpected())

	_, err := svc.Create(context.Background(), agent.CreateRequest{OrgID: uuid.New(), ProjectID: uuid.New(), Slug: "ab", Name: "Acme"})
	assert.True(t, agent.IsInvalidSlugError(err))
	assert.Nil(t, store.Saved)
}

func TestService_Create_StoreError(t *testing.T) {
	store := &fakes.AgentStore{Err: &agent.AlreadyExistsError{Slug: "acme"}}
	svc := agent.NewService(store, fakes.Transactor{}, testUnexpected())

	_, err := svc.Create(context.Background(), agent.CreateRequest{OrgID: uuid.New(), ProjectID: uuid.New(), Slug: "acme", Name: "Acme"})
	assert.True(t, agent.IsAlreadyExistsError(err))
}

func TestService_GetAndList(t *testing.T) {
	a, err := agent.New(uuid.New(), uuid.New(), "acme", "Acme", "", uuid.New())
	require.NoError(t, err)
	store := &fakes.AgentStore{Agent: a, Agents: []*agent.Agent{a}}
	svc := agent.NewService(store, fakes.Transactor{}, testUnexpected())

	got, err := svc.Get(context.Background(), a.ID)
	require.NoError(t, err)
	assert.Equal(t, a, got)

	all, err := svc.List(context.Background(), a.ProjectID)
	require.NoError(t, err)
	assert.Equal(t, []*agent.Agent{a}, all)
}

func TestService_Get_StoreError(t *testing.T) {
	svc := agent.NewService(&fakes.AgentStore{Err: errors.New("boom")}, fakes.Transactor{}, testUnexpected())

	_, err := svc.Get(context.Background(), uuid.New())
	assert.Error(t, err)
	_, err = svc.List(context.Background(), uuid.New())
	assert.Error(t, err)
}

func TestService_Unexpected(t *testing.T) {
	svc := agent.NewService(&fakes.AgentStore{Err: errors.New("boom")}, fakes.Transactor{}, testUnexpected())

	_, err := svc.Get(context.Background(), uuid.New())
	_, ok := apperror.AsAppError(err)
	assert.True(t, ok)
}

func TestService_Update(t *testing.T) {
	a, err := agent.New(uuid.New(), uuid.New(), "acme", "Acme", "", uuid.New())
	require.NoError(t, err)
	store := &fakes.AgentStore{Agent: a}

	got, err := agent.NewService(store, fakes.Transactor{}, testUnexpected()).Update(context.Background(), a.ID, "Acme Two", "new")
	require.NoError(t, err)
	assert.Equal(t, "Acme Two", got.Name)
	assert.Equal(t, "new", got.Description)
	assert.Equal(t, got, store.Saved)
}

func TestService_Update_NotFound(t *testing.T) {
	store := &fakes.AgentStore{Err: &agent.NotFoundError{ID: "x"}}

	_, err := agent.NewService(store, fakes.Transactor{}, testUnexpected()).Update(context.Background(), uuid.New(), "Acme Two", "")
	assert.Error(t, err)
}

func TestService_Update_InvalidName(t *testing.T) {
	a, err := agent.New(uuid.New(), uuid.New(), "acme", "Acme", "", uuid.New())
	require.NoError(t, err)

	_, err = agent.NewService(&fakes.AgentStore{Agent: a}, fakes.Transactor{}, testUnexpected()).Update(context.Background(), a.ID, "", "")
	assert.True(t, agent.IsInvalidNameError(err))
}

func TestService_Delete(t *testing.T) {
	store := &fakes.AgentStore{}
	id := uuid.New()

	require.NoError(t, agent.NewService(store, fakes.Transactor{}, testUnexpected()).Delete(context.Background(), id))
	assert.Equal(t, id, store.Deleted)
}

func TestService_Delete_Error(t *testing.T) {
	store := &fakes.AgentStore{Err: &agent.NotFoundError{ID: "x"}}

	err := agent.NewService(store, fakes.Transactor{}, testUnexpected()).Delete(context.Background(), uuid.New())
	assert.True(t, agent.IsNotFoundError(err))
}

type projectOrgs map[uuid.UUID]uuid.UUID

func (m projectOrgs) OrgOf(_ context.Context, id uuid.UUID) (uuid.UUID, error) {
	if org, ok := m[id]; ok {
		return org, nil
	}
	return uuid.Nil, &agent.NotFoundError{}
}

func TestService_Create_ProjectMustBelongToOrg(t *testing.T) {
	orgA, orgB, prjB := uuid.New(), uuid.New(), uuid.New()
	store := &fakes.AgentStore{}
	svc := agent.NewService(store, fakes.Transactor{}, nil).WithProjects(projectOrgs{prjB: orgB})

	for _, slug := range []string{"bot", ""} {
		_, err := svc.Create(context.Background(), agent.CreateRequest{OrgID: orgA, ProjectID: prjB, Slug: slug, Name: "Bot"})
		assert.True(t, tenant.IsProjectOutsideOrgError(err), "slug %q: %v", slug, err)
	}
	assert.Nil(t, store.Saved)

	_, err := svc.Create(context.Background(), agent.CreateRequest{OrgID: orgB, ProjectID: prjB, Slug: "bot", Name: "Bot"})
	require.NoError(t, err)
}

func TestService_ByPublicID(t *testing.T) {
	a, err := agent.New(uuid.New(), uuid.New(), "bot", "Bot", "", uuid.Nil)
	require.NoError(t, err)
	got, err := agent.NewService(&fakes.AgentStore{Agent: a}, fakes.Transactor{}, testUnexpected()).ByPublicID(context.Background(), a.PublicID)
	require.NoError(t, err)
	assert.Same(t, a, got)

	_, err = agent.NewService(&fakes.AgentStore{Agent: a}, fakes.Transactor{}, testUnexpected()).ByPublicID(context.Background(), "nope")
	assert.True(t, agent.IsNotFoundError(err))
	_, err = agent.NewService(&fakes.AgentStore{Err: errors.New("down")}, fakes.Transactor{}, testUnexpected()).ByPublicID(context.Background(), a.PublicID)
	_, isApp := apperror.AsAppError(err)
	assert.True(t, isApp)
}
