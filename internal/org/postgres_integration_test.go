//go:build integration

package org_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/testutil/pgtest"
)

func newOrgStore(t *testing.T) (org.Store, db.Pool) {
	t.Helper()
	sqlDB, schemaName := pgtest.Migrated(t)
	pool := db.Pool{W: sqlDB, R: sqlDB}
	return org.NewStore(pool, schemaName, ""), pool
}

func seedUser(t *testing.T, pool db.Pool) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := pool.W.ExecContext(t.Context(),
		`INSERT INTO users (id, public_id, email, created_at, updated_at) VALUES ($1, $2, $3, now(), now())`,
		id, "u-"+id.String()[:8], id.String()+"@example.com")
	require.NoError(t, err)
	return id
}

func newOrg(t *testing.T, slug string, createdBy uuid.UUID) *org.Org {
	t.Helper()
	o, err := org.NewOrg(slug, "Name "+slug, createdBy)
	require.NoError(t, err)
	o.CreatedAt = time.Now().UTC().Truncate(time.Microsecond)
	o.UpdatedAt = o.CreatedAt
	return o
}

func TestOrgStore_RoundTrip(t *testing.T) {
	cases := []struct {
		name        string
		withCreator bool
	}{
		{name: "genesis org has NULL created_by"},
		{name: "created_by set", withCreator: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store, pool := newOrgStore(t)
			ctx := t.Context()
			createdBy := uuid.Nil
			if tc.withCreator {
				createdBy = seedUser(t, pool)
			}
			o := newOrg(t, "acme", createdBy)
			o.CreatedAt = time.Date(2024, 2, 29, 23, 59, 58, 123456000, time.FixedZone("WIB", 7*3600))

			before := time.Now()
			require.NoError(t, store.Save(ctx, o))

			byID, err := store.ByID(ctx, o.ID)
			require.NoError(t, err)
			bySlug, err := store.BySlug(ctx, "acme")
			require.NoError(t, err)
			assert.Equal(t, byID, bySlug)
			byPublicID, err := store.ByPublicID(ctx, o.PublicID)
			require.NoError(t, err)
			assert.Equal(t, byID, byPublicID)

			assert.Equal(t, o.ID, byID.ID)
			assert.Equal(t, o.PublicID, byID.PublicID)
			assert.Equal(t, "acme", byID.Slug)
			assert.Equal(t, "Name acme", byID.Name)
			assert.Equal(t, o.CreatedBy, byID.CreatedBy)
			assert.Equal(t, tc.withCreator, byID.CreatedBy.Valid)
			assert.True(t, o.CreatedAt.Equal(byID.CreatedAt), "created_at %v != %v", byID.CreatedAt, o.CreatedAt)
			assert.WithinDuration(t, before, byID.UpdatedAt, 5*time.Second, "updated_at is stamped by the store")
		})
	}
}

func TestOrgStore_SaveUpdatesMutableColumnsOnly(t *testing.T) {
	store, pool := newOrgStore(t)
	ctx := t.Context()
	creator := seedUser(t, pool)
	o := newOrg(t, "acme", creator)
	require.NoError(t, store.Save(ctx, o))
	first, err := store.ByID(ctx, o.ID)
	require.NoError(t, err)

	changed := *o
	changed.Slug = "acme-renamed"
	changed.Name = "Acme Renamed"
	changed.CreatedBy = uuid.NullUUID{UUID: uuid.New(), Valid: true}
	changed.CreatedAt = o.CreatedAt.Add(-48 * time.Hour)
	require.NoError(t, store.Save(ctx, &changed))

	got, err := store.ByID(ctx, o.ID)
	require.NoError(t, err)
	assert.Equal(t, "acme-renamed", got.Slug)
	assert.Equal(t, "Acme Renamed", got.Name)
	assert.Equal(t, uuid.NullUUID{UUID: creator, Valid: true}, got.CreatedBy, "created_by is immutable")
	assert.True(t, first.CreatedAt.Equal(got.CreatedAt), "created_at is immutable")
	assert.False(t, got.UpdatedAt.Before(first.UpdatedAt), "updated_at must not go backwards")

	_, err = store.BySlug(ctx, "acme")
	assert.True(t, org.IsNotFoundError(err), "old slug must be released, got %v", err)

	all, err := store.List(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 1, "upsert by id must not insert a second row")
}

func TestOrgStore_UniqueConstraints(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(existing, other *org.Org)
	}{
		{name: "slug on insert", mutate: func(existing, other *org.Org) { other.Slug = existing.Slug }},
		{name: "public_id on insert", mutate: func(existing, other *org.Org) { other.PublicID = existing.PublicID }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store, _ := newOrgStore(t)
			ctx := t.Context()
			existing := newOrg(t, "acme", uuid.Nil)
			require.NoError(t, store.Save(ctx, existing))

			other := newOrg(t, "globex", uuid.Nil)
			tc.mutate(existing, other)
			err := store.Save(ctx, other)
			require.Error(t, err)
			assert.True(t, org.IsAlreadyExistsError(err), "want AlreadyExistsError, got %T: %v", err, err)

			_, err = store.ByID(ctx, other.ID)
			assert.True(t, org.IsNotFoundError(err), "a rejected insert must leave no row")
		})
	}

	t.Run("slug on update into another org's slug", func(t *testing.T) {
		store, _ := newOrgStore(t)
		ctx := t.Context()
		a, b := newOrg(t, "acme", uuid.Nil), newOrg(t, "globex", uuid.Nil)
		require.NoError(t, store.Save(ctx, a))
		require.NoError(t, store.Save(ctx, b))

		b.Slug = "acme"
		err := store.Save(ctx, b)
		assert.True(t, org.IsAlreadyExistsError(err), "want AlreadyExistsError, got %T: %v", err, err)

		got, err := store.ByID(ctx, b.ID)
		require.NoError(t, err)
		assert.Equal(t, "globex", got.Slug, "the failed update must not change the row")
	})
}

func TestOrgStore_NotFound(t *testing.T) {
	store, _ := newOrgStore(t)
	ctx := t.Context()
	require.NoError(t, store.Save(ctx, newOrg(t, "acme", uuid.Nil)))

	id := uuid.New()
	_, err := store.ByID(ctx, id)
	nf, ok := errors.AsType[*org.NotFoundError](err)
	require.True(t, ok, "want NotFoundError, got %T: %v", err, err)
	assert.Equal(t, id.String(), nf.ID)

	_, err = store.BySlug(ctx, "missing")
	nf, ok = errors.AsType[*org.NotFoundError](err)
	require.True(t, ok, "want NotFoundError, got %T: %v", err, err)
	assert.Equal(t, "missing", nf.Slug)

	_, err = store.BySlug(ctx, "ACME")
	assert.True(t, org.IsNotFoundError(err), "slug lookup is case sensitive")

	_, err = store.ByPublicID(ctx, "missing")
	nf, ok = errors.AsType[*org.NotFoundError](err)
	require.True(t, ok, "want NotFoundError, got %T: %v", err, err)
	assert.Empty(t, nf.ID+nf.Slug, "an unknown public id is not echoed")
}

func TestOrgStore_ListOrdersByCreatedAtAscending(t *testing.T) {
	store, _ := newOrgStore(t)
	ctx := t.Context()

	empty, err := store.List(ctx)
	require.NoError(t, err)
	assert.Empty(t, empty)

	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	offsets := map[string]time.Duration{"charlie": 3 * time.Hour, "alpha": time.Hour, "bravo": 2 * time.Hour}
	for _, slug := range []string{"charlie", "alpha", "bravo"} {
		o := newOrg(t, slug, uuid.Nil)
		o.CreatedAt = base.Add(offsets[slug])
		require.NoError(t, store.Save(ctx, o))
	}

	got, err := store.List(ctx)
	require.NoError(t, err)
	slugs := make([]string, 0, len(got))
	for _, o := range got {
		slugs = append(slugs, o.Slug)
	}
	assert.Equal(t, []string{"alpha", "bravo", "charlie"}, slugs)
}

func TestOrgStore_HonorsUnitOfWork(t *testing.T) {
	errAbort := errors.New("abort")

	t.Run("rollback leaves nothing", func(t *testing.T) {
		store, pool := newOrgStore(t)
		o := newOrg(t, "acme", uuid.Nil)
		err := db.RunInTx(t.Context(), pool, func(ctx context.Context) error {
			require.NoError(t, store.Save(ctx, o))
			got, err := store.ByID(ctx, o.ID)
			require.NoError(t, err, "reads inside the unit of work see its own writes")
			assert.Equal(t, o.ID, got.ID)

			_, err = store.ByID(t.Context(), o.ID)
			assert.True(t, org.IsNotFoundError(err), "uncommitted row must be invisible outside the tx")
			return errAbort
		})
		require.ErrorIs(t, err, errAbort)

		_, err = store.ByID(t.Context(), o.ID)
		assert.True(t, org.IsNotFoundError(err), "rolled-back row must not persist, got %v", err)
	})

	t.Run("commit persists", func(t *testing.T) {
		store, pool := newOrgStore(t)
		o := newOrg(t, "acme", uuid.Nil)
		require.NoError(t, db.RunInTx(t.Context(), pool, func(ctx context.Context) error {
			return store.Save(ctx, o)
		}))
		got, err := store.ByID(t.Context(), o.ID)
		require.NoError(t, err)
		assert.Equal(t, "acme", got.Slug)
	})

	t.Run("unique violation inside tx is still typed", func(t *testing.T) {
		store, pool := newOrgStore(t)
		require.NoError(t, store.Save(t.Context(), newOrg(t, "acme", uuid.Nil)))
		err := db.RunInTx(t.Context(), pool, func(ctx context.Context) error {
			return store.Save(ctx, newOrg(t, "acme", uuid.Nil))
		})
		assert.True(t, org.IsAlreadyExistsError(err), "want AlreadyExistsError, got %T: %v", err, err)
	})
}

func TestOrgStore_ConcurrentSameSlugExactlyOneWins(t *testing.T) {
	store, _ := newOrgStore(t)
	ctx := t.Context()
	const workers = 8

	start := make(chan struct{})
	errs := make([]error, workers)
	var wg sync.WaitGroup
	for i := range workers {
		o := newOrg(t, "contested", uuid.Nil)
		wg.Go(func() {
			<-start
			errs[i] = store.Save(ctx, o)
		})
	}
	close(start)
	wg.Wait()

	var ok, dup int
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case org.IsAlreadyExistsError(err):
			dup++
		default:
			t.Errorf("unexpected error: %T: %v", err, err)
		}
	}
	assert.Equal(t, 1, ok)
	assert.Equal(t, workers-1, dup)

	all, err := store.List(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 1)
}
