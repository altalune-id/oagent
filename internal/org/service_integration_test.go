//go:build integration

package org_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/org"
)

func TestService_CreateWithAFailedHookLeavesNoOrg(t *testing.T) {
	for _, slug := range []string{"acme", ""} {
		t.Run("slug="+slug, func(t *testing.T) {
			store, pool := newOrgStore(t)
			var hookInTx bool
			svc := org.NewService(store, db.NewTransactor(pool), apperror.NewReporter(nil, false).Unexpected).
				WithAfterCreate(func(ctx context.Context, _ *org.Org) error {
					_, hookInTx = db.CurrentTx(ctx)
					return errors.New("default project failed")
				})

			_, err := svc.Create(t.Context(), org.CreateRequest{Slug: slug, Name: "Acme"})
			require.Error(t, err)
			assert.True(t, hookInTx, "the hook runs in the org's unit of work")
			orgs, lerr := store.List(t.Context())
			require.NoError(t, lerr)
			assert.Empty(t, orgs, "an org never exists without what its hook adds")
		})
	}
}

func TestService_CreateRunsTheHookInTheSameUnitOfWork(t *testing.T) {
	store, pool := newOrgStore(t)
	var hookInTx bool
	svc := org.NewService(store, db.NewTransactor(pool), apperror.NewReporter(nil, false).Unexpected).
		WithAfterCreate(func(ctx context.Context, _ *org.Org) error {
			_, hookInTx = db.CurrentTx(ctx)
			return nil
		})

	o, err := svc.Create(t.Context(), org.CreateRequest{Name: "Acme"})
	require.NoError(t, err)
	assert.True(t, hookInTx)
	_, err = store.ByID(t.Context(), o.ID)
	require.NoError(t, err)
}
