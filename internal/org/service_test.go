package org_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/testutil/fakes"
)

func testUnexpected() apperror.UnexpectedFunc {
	return apperror.NewReporter(nil, false).Unexpected
}

func TestService_Create(t *testing.T) {
	store := &fakes.OrgStore{}
	svc := org.NewService(store, fakes.Transactor{}, testUnexpected())

	o, err := svc.Create(context.Background(), org.CreateRequest{Slug: "acme", Name: "Acme", CreatedBy: uuid.New()})
	require.NoError(t, err)
	assert.Equal(t, o, store.Saved)
	assert.Equal(t, "acme", o.Slug)
}

func TestService_Create_GeneratedSlug(t *testing.T) {
	svc := org.NewService(&fakes.OrgStore{}, fakes.Transactor{}, testUnexpected())

	o, err := svc.Create(context.Background(), org.CreateRequest{Name: "Acme"})
	require.NoError(t, err)
	assert.NotEmpty(t, o.Slug)
	assert.NoError(t, org.ValidateSlug(o.Slug))
}

func TestService_Create_GeneratedSlugRetries(t *testing.T) {
	calls := 0
	store := &fakes.OrgStore{SaveFn: func(*org.Org) error {
		calls++
		if calls < 3 {
			return &org.AlreadyExistsError{}
		}
		return nil
	}}

	o, err := org.NewService(store, fakes.Transactor{}, testUnexpected()).Create(context.Background(), org.CreateRequest{Name: "Acme"})
	require.NoError(t, err)
	assert.Equal(t, 3, calls)
	assert.NotEmpty(t, o.Slug)
}

func TestService_Create_GeneratedSlugExhausted(t *testing.T) {
	store := &fakes.OrgStore{SaveFn: func(*org.Org) error { return &org.AlreadyExistsError{} }}

	_, err := org.NewService(store, fakes.Transactor{}, testUnexpected()).Create(context.Background(), org.CreateRequest{Name: "Acme"})
	assert.True(t, org.IsAlreadyExistsError(err))
}

func TestService_Create_Validation(t *testing.T) {
	store := &fakes.OrgStore{}
	svc := org.NewService(store, fakes.Transactor{}, testUnexpected())

	_, err := svc.Create(context.Background(), org.CreateRequest{Slug: "ab", Name: "Acme"})
	assert.True(t, org.IsInvalidSlugError(err))
	assert.Nil(t, store.Saved)
}

func TestService_Create_StoreError(t *testing.T) {
	store := &fakes.OrgStore{Err: &org.AlreadyExistsError{Slug: "acme"}}
	svc := org.NewService(store, fakes.Transactor{}, testUnexpected())

	_, err := svc.Create(context.Background(), org.CreateRequest{Slug: "acme", Name: "Acme"})
	assert.True(t, org.IsAlreadyExistsError(err))
}

func TestService_GetAndList(t *testing.T) {
	o, err := org.NewOrg("acme", "Acme", uuid.New())
	require.NoError(t, err)
	store := &fakes.OrgStore{Org: o, Orgs: []*org.Org{o}}
	svc := org.NewService(store, fakes.Transactor{}, testUnexpected())

	got, err := svc.Get(context.Background(), o.ID)
	require.NoError(t, err)
	assert.Equal(t, o, got)

	bySlug, err := svc.BySlug(context.Background(), "acme")
	require.NoError(t, err)
	assert.Equal(t, o, bySlug)

	byPublicID, err := svc.ByPublicID(context.Background(), o.PublicID)
	require.NoError(t, err)
	assert.Equal(t, o, byPublicID)

	all, err := svc.List(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []*org.Org{o}, all)
}

func TestService_Get_StoreError(t *testing.T) {
	svc := org.NewService(&fakes.OrgStore{Err: errors.New("boom")}, fakes.Transactor{}, testUnexpected())

	_, err := svc.Get(context.Background(), uuid.New())
	assert.Error(t, err)
	_, err = svc.BySlug(context.Background(), "acme")
	assert.Error(t, err)
	_, err = svc.ByPublicID(context.Background(), "p")
	assert.Error(t, err)
	_, err = svc.List(context.Background())
	assert.Error(t, err)
}

func TestService_Unexpected(t *testing.T) {
	svc := org.NewService(&fakes.OrgStore{Err: errors.New("boom")}, fakes.Transactor{}, testUnexpected())

	_, err := svc.Get(context.Background(), uuid.New())
	_, ok := apperror.AsAppError(err)
	assert.True(t, ok)
}

func TestService_Update(t *testing.T) {
	o, err := org.NewOrg("acme", "Acme", uuid.New())
	require.NoError(t, err)
	store := &fakes.OrgStore{Org: o}

	got, err := org.NewService(store, fakes.Transactor{}, testUnexpected()).Update(context.Background(), o.ID, "Acme Two", "acme-two")
	require.NoError(t, err)
	assert.Equal(t, "Acme Two", got.Name)
	assert.Equal(t, "acme-two", got.Slug)
	assert.Equal(t, got, store.Saved)
}

func TestService_Update_NotFound(t *testing.T) {
	store := &fakes.OrgStore{Err: &org.NotFoundError{ID: "x"}}

	_, err := org.NewService(store, fakes.Transactor{}, testUnexpected()).Update(context.Background(), uuid.New(), "Acme Two", "acme-two")
	assert.Error(t, err)
}

func TestService_Update_InvalidName(t *testing.T) {
	o, err := org.NewOrg("acme", "Acme", uuid.New())
	require.NoError(t, err)

	_, err = org.NewService(&fakes.OrgStore{Org: o}, fakes.Transactor{}, testUnexpected()).Update(context.Background(), o.ID, "", "acme-two")
	assert.True(t, org.IsInvalidNameError(err))
}

func TestService_Update_InvalidSlug(t *testing.T) {
	o, err := org.NewOrg("acme", "Acme", uuid.New())
	require.NoError(t, err)

	_, err = org.NewService(&fakes.OrgStore{Org: o}, fakes.Transactor{}, testUnexpected()).Update(context.Background(), o.ID, "Acme", "Bad_Slug")
	assert.True(t, org.IsInvalidSlugError(err))
}

func TestService_Create_AfterCreateHook(t *testing.T) {
	var hooked *org.Org
	svc := org.NewService(&fakes.OrgStore{}, fakes.Transactor{}, testUnexpected()).WithAfterCreate(func(_ context.Context, o *org.Org) error {
		hooked = o
		return nil
	})

	o, err := svc.Create(context.Background(), org.CreateRequest{Slug: "acme", Name: "Acme"})
	require.NoError(t, err)
	assert.Equal(t, o, hooked)
}

func TestService_Create_AfterCreateHookError(t *testing.T) {
	svc := org.NewService(&fakes.OrgStore{}, fakes.Transactor{}, testUnexpected()).WithAfterCreate(func(context.Context, *org.Org) error {
		return &org.AlreadyExistsError{Slug: "default"}
	})

	_, err := svc.Create(context.Background(), org.CreateRequest{Slug: "acme", Name: "Acme"})
	assert.True(t, org.IsAlreadyExistsError(err))
}

func TestService_CreationGate(t *testing.T) {
	store := &fakes.OrgStore{}
	svc := org.NewService(store, fakes.Transactor{}, testUnexpected()).WithCreation(false)
	_, err := svc.Create(context.Background(), org.CreateRequest{Slug: "acme", Name: "Acme"})
	assert.True(t, org.IsCreationDisabledError(err), "got %v", err)
	assert.Nil(t, store.Saved, "nothing saved when creation is off")

	svc = org.NewService(&fakes.OrgStore{}, fakes.Transactor{}, testUnexpected())
	_, err = svc.Create(context.Background(), org.CreateRequest{Slug: "acme", Name: "Acme"})
	assert.NoError(t, err, "default stays allowed until boot passes capabilities")
}

func TestService_System(t *testing.T) {
	sys, err := org.NewSystemOrg("acme", "Acme", uuid.Nil)
	require.NoError(t, err)
	svc := org.NewService(&fakes.OrgStore{Org: sys}, fakes.Transactor{}, testUnexpected())
	got, err := svc.System(context.Background())
	require.NoError(t, err)
	assert.True(t, got.IsSystem)

	svc = org.NewService(&fakes.OrgStore{}, fakes.Transactor{}, testUnexpected())
	_, err = svc.System(context.Background())
	assert.True(t, org.IsNotFoundError(err), "got %v", err)

	svc = org.NewService(&fakes.OrgStore{Err: errors.New("db down")}, fakes.Transactor{}, testUnexpected())
	_, err = svc.System(context.Background())
	ae, ok := errors.AsType[*apperror.AppError](err)
	require.True(t, ok, "got %v", err)
	assert.Equal(t, apperror.CodeUnexpected, ae.Code())
}

func TestService_CreateSystem_IgnoresGateAndHook(t *testing.T) {
	store := &fakes.OrgStore{}
	hookCalls := 0
	svc := org.NewService(store, fakes.Transactor{}, testUnexpected()).
		WithCreation(false).
		WithAfterCreate(func(context.Context, *org.Org) error { hookCalls++; return nil })
	owner := uuid.New()

	o, err := svc.CreateSystem(context.Background(), org.CreateRequest{Name: "Default", CreatedBy: owner})
	require.NoError(t, err)
	assert.True(t, o.IsSystem)
	assert.Equal(t, o, store.Saved)
	assert.Equal(t, owner, o.CreatedBy.UUID)
	assert.NoError(t, org.ValidateSlug(o.Slug))
	assert.Zero(t, hookCalls)

	_, err = svc.Create(context.Background(), org.CreateRequest{Name: "Other"})
	assert.True(t, org.IsCreationDisabledError(err), "the gate still applies to Create")
}

func TestService_CreateSystem_ExplicitSlug(t *testing.T) {
	store := &fakes.OrgStore{}
	o, err := org.NewService(store, fakes.Transactor{}, testUnexpected()).CreateSystem(context.Background(), org.CreateRequest{Slug: "acme", Name: "Acme"})
	require.NoError(t, err)
	assert.Equal(t, "acme", o.Slug)
	assert.False(t, o.CreatedBy.Valid)
}
