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
	svc := org.NewService(store, testUnexpected())

	o, err := svc.Create(context.Background(), org.CreateRequest{Slug: "acme", Name: "Acme", CreatedBy: uuid.New()})
	require.NoError(t, err)
	assert.Equal(t, o, store.Saved)
	assert.Equal(t, "acme", o.Slug)
}

func TestService_Create_GeneratedSlug(t *testing.T) {
	svc := org.NewService(&fakes.OrgStore{}, testUnexpected())

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

	o, err := org.NewService(store, testUnexpected()).Create(context.Background(), org.CreateRequest{Name: "Acme"})
	require.NoError(t, err)
	assert.Equal(t, 3, calls)
	assert.NotEmpty(t, o.Slug)
}

func TestService_Create_GeneratedSlugExhausted(t *testing.T) {
	store := &fakes.OrgStore{SaveFn: func(*org.Org) error { return &org.AlreadyExistsError{} }}

	_, err := org.NewService(store, testUnexpected()).Create(context.Background(), org.CreateRequest{Name: "Acme"})
	assert.True(t, org.IsAlreadyExistsError(err))
}

func TestService_Create_Validation(t *testing.T) {
	store := &fakes.OrgStore{}
	svc := org.NewService(store, testUnexpected())

	_, err := svc.Create(context.Background(), org.CreateRequest{Slug: "ab", Name: "Acme"})
	assert.True(t, org.IsInvalidSlugError(err))
	assert.Nil(t, store.Saved)
}

func TestService_Create_StoreError(t *testing.T) {
	store := &fakes.OrgStore{Err: &org.AlreadyExistsError{Slug: "acme"}}
	svc := org.NewService(store, testUnexpected())

	_, err := svc.Create(context.Background(), org.CreateRequest{Slug: "acme", Name: "Acme"})
	assert.True(t, org.IsAlreadyExistsError(err))
}

func TestService_GetAndList(t *testing.T) {
	o, err := org.NewOrg("acme", "Acme", uuid.New())
	require.NoError(t, err)
	store := &fakes.OrgStore{Org: o, Orgs: []*org.Org{o}}
	svc := org.NewService(store, testUnexpected())

	got, err := svc.Get(context.Background(), o.ID)
	require.NoError(t, err)
	assert.Equal(t, o, got)

	bySlug, err := svc.BySlug(context.Background(), "acme")
	require.NoError(t, err)
	assert.Equal(t, o, bySlug)

	all, err := svc.List(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []*org.Org{o}, all)
}

func TestService_Get_StoreError(t *testing.T) {
	svc := org.NewService(&fakes.OrgStore{Err: errors.New("boom")}, testUnexpected())

	_, err := svc.Get(context.Background(), uuid.New())
	assert.Error(t, err)
	_, err = svc.BySlug(context.Background(), "acme")
	assert.Error(t, err)
	_, err = svc.List(context.Background())
	assert.Error(t, err)
}

func TestService_Unexpected(t *testing.T) {
	svc := org.NewService(&fakes.OrgStore{Err: errors.New("boom")}, testUnexpected())

	_, err := svc.Get(context.Background(), uuid.New())
	_, ok := apperror.AsAppError(err)
	assert.True(t, ok)
}

func TestService_Rename(t *testing.T) {
	o, err := org.NewOrg("acme", "Acme", uuid.New())
	require.NoError(t, err)
	store := &fakes.OrgStore{Org: o}

	got, err := org.NewService(store, testUnexpected()).Rename(context.Background(), o.ID, "Acme Two")
	require.NoError(t, err)
	assert.Equal(t, "Acme Two", got.Name)
	assert.Equal(t, got, store.Saved)
}

func TestService_Rename_NotFound(t *testing.T) {
	store := &fakes.OrgStore{Err: &org.NotFoundError{ID: "x"}}

	_, err := org.NewService(store, testUnexpected()).Rename(context.Background(), uuid.New(), "Acme Two")
	assert.Error(t, err)
}

func TestService_Rename_InvalidName(t *testing.T) {
	o, err := org.NewOrg("acme", "Acme", uuid.New())
	require.NoError(t, err)

	_, err = org.NewService(&fakes.OrgStore{Org: o}, testUnexpected()).Rename(context.Background(), o.ID, "")
	assert.True(t, org.IsInvalidNameError(err))
}
