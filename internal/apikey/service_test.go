package apikey_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/apikey"
	"altalune.id/oagent/internal/testutil/fakes"
	"altalune.id/oagent/scope"
)

type stubProjects struct {
	ids []uuid.UUID
	err error
}

func (s stubProjects) ProjectIDs(context.Context, uuid.UUID) ([]uuid.UUID, error) {
	return s.ids, s.err
}

var knownProject = uuid.MustParse("0b7d1c7e-5a3f-4e2b-9c11-2f6a8d4e7b01")

func future() *time.Time {
	t := time.Now().UTC().Add(time.Hour)
	return &t
}

func testUnexpected() apperror.UnexpectedFunc {
	return apperror.NewReporter(nil, false).Unexpected
}

func newService(store apikey.Store, projects apikey.Projects) *apikey.Service {
	return apikey.NewService(store, apikey.NewScheme(""), projects, testUnexpected())
}

func TestService_Create_Project(t *testing.T) {
	store := &fakes.APIKeyStore{}
	k, plaintext, err := newService(store, stubProjects{ids: []uuid.UUID{knownProject}}).Create(context.Background(), apikey.CreateRequest{
		OrgID: uuid.New(), ProjectID: knownProject, Kind: apikey.KindProject, Name: "ci",
		Scopes: []string{scope.AgentRead}, ExpiresAt: future(),
	})
	require.NoError(t, err)
	assert.NotEmpty(t, plaintext)
	assert.Equal(t, k, store.Saved)
	assert.Equal(t, apikey.KindProject, k.Kind)
}

func TestService_Create_Org(t *testing.T) {
	projectID := uuid.New()
	store := &fakes.APIKeyStore{}
	k, _, err := newService(store, stubProjects{ids: []uuid.UUID{projectID}}).Create(context.Background(), apikey.CreateRequest{
		OrgID: uuid.New(), Kind: apikey.KindOrg, Name: "deploy",
		Scopes: []string{scope.OrgRead}, ProjectIDs: []uuid.UUID{projectID}, ExpiresAt: future(),
	})
	require.NoError(t, err)
	assert.True(t, k.HasGrant())
	assert.Equal(t, []uuid.UUID{projectID}, k.ProjectIDs)
}

func TestService_Create_Org_AllProjects(t *testing.T) {
	_, _, err := newService(&fakes.APIKeyStore{}, stubProjects{}).Create(context.Background(), apikey.CreateRequest{
		OrgID: uuid.New(), Kind: apikey.KindOrg, Name: "deploy", AllProjects: true, ExpiresAt: future(),
	})
	require.NoError(t, err)
}

func TestService_Create_Validation(t *testing.T) {
	ctx := context.Background()

	_, _, err := newService(&fakes.APIKeyStore{}, stubProjects{}).Create(ctx, apikey.CreateRequest{Kind: apikey.KindOrg, Name: "x", AllProjects: true, ExpiresAt: future()})
	_, ok := apperror.AsAppError(err)
	assert.True(t, ok, "missing org")

	_, _, err = newService(&fakes.APIKeyStore{}, stubProjects{}).Create(ctx, apikey.CreateRequest{OrgID: uuid.New(), Kind: apikey.KindProject, Name: "x", ExpiresAt: future()})
	_, ok = apperror.AsAppError(err)
	assert.True(t, ok, "project key needs project id")

	_, _, err = newService(&fakes.APIKeyStore{}, stubProjects{}).Create(ctx, apikey.CreateRequest{OrgID: uuid.New(), Kind: "bogus", Name: "x", ExpiresAt: future()})
	_, ok = apperror.AsAppError(err)
	assert.True(t, ok, "invalid kind")
}

func TestService_Create_GrantProjectNotInOrg(t *testing.T) {
	_, _, err := newService(&fakes.APIKeyStore{}, stubProjects{ids: []uuid.UUID{uuid.New()}}).Create(context.Background(), apikey.CreateRequest{
		OrgID: uuid.New(), Kind: apikey.KindOrg, Name: "x", ProjectIDs: []uuid.UUID{uuid.New()}, ExpiresAt: future(),
	})
	_, ok := apperror.AsAppError(err)
	assert.True(t, ok)
}

func TestService_Create_GrantProjectsError(t *testing.T) {
	_, _, err := newService(&fakes.APIKeyStore{}, stubProjects{err: errors.New("boom")}).Create(context.Background(), apikey.CreateRequest{
		OrgID: uuid.New(), Kind: apikey.KindOrg, Name: "x", ProjectIDs: []uuid.UUID{uuid.New()}, ExpiresAt: future(),
	})
	assert.Error(t, err)
}

func TestService_Create_UnknownScope(t *testing.T) {
	_, _, err := newService(&fakes.APIKeyStore{}, stubProjects{}).Create(context.Background(), apikey.CreateRequest{
		OrgID: uuid.New(), Kind: apikey.KindOrg, Name: "x", Scopes: []string{"nope"}, AllProjects: true, ExpiresAt: future(),
	})
	assert.IsType(t, &apikey.UnknownScopeError{}, err)
}

func TestService_Create_StoreError(t *testing.T) {
	_, _, err := newService(&fakes.APIKeyStore{Err: errors.New("boom")}, stubProjects{}).Create(context.Background(), apikey.CreateRequest{
		OrgID: uuid.New(), Kind: apikey.KindOrg, Name: "x", AllProjects: true, ExpiresAt: future(),
	})
	_, ok := apperror.AsAppError(err)
	assert.True(t, ok)
}

func TestService_List(t *testing.T) {
	k, _, err := apikey.NewScheme("").MintOrg(uuid.New(), "x", nil, apikey.ProjectGrant{All: true}, future(), time.Now().UTC())
	require.NoError(t, err)
	store := &fakes.APIKeyStore{Keys: []*apikey.APIKey{k}}
	got, err := newService(store, stubProjects{}).List(context.Background(), k.OrgID, uuid.Nil)
	require.NoError(t, err)
	assert.Equal(t, []*apikey.APIKey{k}, got)
}

func TestService_List_Error(t *testing.T) {
	_, err := newService(&fakes.APIKeyStore{Err: errors.New("boom")}, stubProjects{}).List(context.Background(), uuid.New(), uuid.Nil)
	assert.Error(t, err)
}

func TestService_Revoke(t *testing.T) {
	orgID := uuid.New()
	k, _, err := apikey.NewScheme("").MintOrg(orgID, "x", nil, apikey.ProjectGrant{All: true}, future(), time.Now().UTC())
	require.NoError(t, err)
	store := &fakes.APIKeyStore{Key: k}
	require.NoError(t, newService(store, stubProjects{}).Revoke(context.Background(), orgID, k.ID, uuid.Nil))
	assert.NotNil(t, store.Saved.RevokedAt)
}

func TestService_Revoke_WrongOrg(t *testing.T) {
	k, _, err := apikey.NewScheme("").MintOrg(uuid.New(), "x", nil, apikey.ProjectGrant{All: true}, future(), time.Now().UTC())
	require.NoError(t, err)
	err = newService(&fakes.APIKeyStore{Key: k}, stubProjects{}).Revoke(context.Background(), uuid.New(), k.ID, uuid.Nil)
	assert.IsType(t, &apikey.NotFoundError{}, err)
}

func TestService_Revoke_NotFound(t *testing.T) {
	store := &fakes.APIKeyStore{Err: &apikey.NotFoundError{ID: "x"}}
	err := newService(store, stubProjects{}).Revoke(context.Background(), uuid.New(), uuid.New(), uuid.Nil)
	assert.True(t, apikey.IsNotFoundError(err))
}

func TestService_Revoke_SaveError(t *testing.T) {
	orgID := uuid.New()
	k, _, err := apikey.NewScheme("").MintOrg(orgID, "x", nil, apikey.ProjectGrant{All: true}, future(), time.Now().UTC())
	require.NoError(t, err)
	store := &fakes.APIKeyStore{Key: k, SaveFn: func(*apikey.APIKey) error { return errors.New("boom") }}

	err = newService(store, stubProjects{}).Revoke(context.Background(), orgID, k.ID, uuid.Nil)
	_, ok := apperror.AsAppError(err)
	assert.True(t, ok)
}

func TestService_Create_ProjectKeyProjectMustBelongToOrg(t *testing.T) {
	mine := uuid.New()
	store := &fakes.APIKeyStore{}
	svc := newService(store, stubProjects{ids: []uuid.UUID{mine}})

	_, _, err := svc.Create(context.Background(), apikey.CreateRequest{
		OrgID: uuid.New(), ProjectID: uuid.New(), Kind: apikey.KindProject, Name: "ci", Scopes: []string{scope.AgentRead}, ExpiresAt: future(),
	})
	ae, ok := apperror.AsAppError(err)
	require.True(t, ok, "got %v", err)
	assert.Equal(t, apperror.CodeAPIKeyInvalidGrant, ae.Code())
	assert.Nil(t, store.Saved)

	_, _, err = svc.Create(context.Background(), apikey.CreateRequest{
		OrgID: uuid.New(), ProjectID: mine, Kind: apikey.KindProject, Name: "ci", Scopes: []string{scope.AgentRead}, ExpiresAt: future(),
	})
	require.NoError(t, err)
}
