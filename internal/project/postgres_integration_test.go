//go:build integration

package project_test

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/project"
	"altalune.id/oagent/internal/testutil/pgtest"
)

type projectFixture struct {
	sqlDB    *sql.DB
	pool     db.Pool
	orgs     org.Store
	projects project.Store
}

func newProjectFixture(t *testing.T) *projectFixture {
	t.Helper()
	sqlDB, schemaName := pgtest.Migrated(t)
	pool := db.Pool{W: sqlDB, R: sqlDB}
	return &projectFixture{
		sqlDB:    sqlDB,
		pool:     pool,
		orgs:     org.NewStore(pool, schemaName, ""),
		projects: project.NewStore(pool, schemaName, ""),
	}
}

func (f *projectFixture) seedOrg(t *testing.T, slug string) uuid.UUID {
	t.Helper()
	o, err := org.NewOrg(slug, "Org "+slug, uuid.Nil)
	require.NoError(t, err)
	require.NoError(t, f.orgs.Save(t.Context(), o))
	return o.ID
}

func newProject(t *testing.T, orgID uuid.UUID, slug string, createdBy uuid.UUID) *project.Project {
	t.Helper()
	p, err := project.NewProject(orgID, slug, "Project "+slug, createdBy)
	require.NoError(t, err)
	p.CreatedAt = time.Now().UTC().Truncate(time.Microsecond)
	p.UpdatedAt = p.CreatedAt
	return p
}

func slugsOf(ps []*project.Project) []string {
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		out = append(out, p.Slug)
	}
	return out
}

func TestProjectStore_RoundTrip(t *testing.T) {
	cases := []struct {
		name      string
		createdBy uuid.UUID
	}{
		{name: "created_by NULL", createdBy: uuid.Nil},
		{name: "created_by set", createdBy: uuid.New()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newProjectFixture(t)
			ctx := t.Context()
			orgID := f.seedOrg(t, "acme")
			p := newProject(t, orgID, "web", tc.createdBy)
			p.CreatedAt = time.Date(2024, 2, 29, 23, 59, 58, 123456000, time.FixedZone("WIB", 7*3600))

			before := time.Now()
			require.NoError(t, f.projects.Save(ctx, p))

			byID, err := f.projects.ByID(ctx, p.ID)
			require.NoError(t, err)
			bySlug, err := f.projects.BySlug(ctx, orgID, "web")
			require.NoError(t, err)
			assert.Equal(t, byID, bySlug)

			assert.Equal(t, p.ID, byID.ID)
			assert.Equal(t, p.PublicID, byID.PublicID)
			assert.Equal(t, orgID, byID.OrgID)
			assert.Equal(t, "web", byID.Slug)
			assert.Equal(t, "Project web", byID.Name)
			assert.Equal(t, p.CreatedBy, byID.CreatedBy)
			assert.Equal(t, tc.createdBy != uuid.Nil, byID.CreatedBy.Valid)
			assert.True(t, p.CreatedAt.Equal(byID.CreatedAt), "created_at %v != %v", byID.CreatedAt, p.CreatedAt)
			assert.WithinDuration(t, before, byID.UpdatedAt, 5*time.Second, "updated_at is stamped by the store")
		})
	}
}

func TestProjectStore_SaveUpdatesMutableColumnsOnly(t *testing.T) {
	f := newProjectFixture(t)
	ctx := t.Context()
	orgA, orgB := f.seedOrg(t, "acme"), f.seedOrg(t, "globex")
	creator := uuid.New()
	p := newProject(t, orgA, "web", creator)
	require.NoError(t, f.projects.Save(ctx, p))
	first, err := f.projects.ByID(ctx, p.ID)
	require.NoError(t, err)

	changed := *p
	changed.Slug = "web-renamed"
	changed.Name = "Web Renamed"
	changed.OrgID = orgB
	changed.PublicID = "different-public-id"
	changed.CreatedBy = uuid.NullUUID{UUID: uuid.New(), Valid: true}
	changed.CreatedAt = p.CreatedAt.Add(-48 * time.Hour)
	require.NoError(t, f.projects.Save(ctx, &changed))

	got, err := f.projects.ByID(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, "web-renamed", got.Slug)
	assert.Equal(t, "Web Renamed", got.Name)
	assert.Equal(t, orgA, got.OrgID, "org_id is immutable: a project never moves between orgs")
	assert.Equal(t, p.PublicID, got.PublicID, "public_id is immutable")
	assert.Equal(t, uuid.NullUUID{UUID: creator, Valid: true}, got.CreatedBy, "created_by is immutable")
	assert.True(t, first.CreatedAt.Equal(got.CreatedAt), "created_at is immutable")
	assert.False(t, got.UpdatedAt.Before(first.UpdatedAt), "updated_at must not go backwards")

	inB, err := f.projects.List(ctx, orgB)
	require.NoError(t, err)
	assert.Empty(t, inB)
	inA, err := f.projects.List(ctx, orgA)
	require.NoError(t, err)
	assert.Equal(t, []string{"web-renamed"}, slugsOf(inA), "upsert by id must not insert a second row")
}

func TestProjectStore_UniqueConstraints(t *testing.T) {
	t.Run("slug unique within an org", func(t *testing.T) {
		f := newProjectFixture(t)
		ctx := t.Context()
		orgID := f.seedOrg(t, "acme")
		require.NoError(t, f.projects.Save(ctx, newProject(t, orgID, "web", uuid.Nil)))

		dup := newProject(t, orgID, "web", uuid.Nil)
		err := f.projects.Save(ctx, dup)
		ae, ok := errors.AsType[*project.AlreadyExistsError](err)
		require.True(t, ok, "want AlreadyExistsError, got %T: %v", err, err)
		assert.Equal(t, "web", ae.Slug)
		_, err = f.projects.ByID(ctx, dup.ID)
		assert.True(t, project.IsNotFoundError(err), "a rejected insert must leave no row")
	})

	t.Run("same slug allowed in different orgs", func(t *testing.T) {
		f := newProjectFixture(t)
		ctx := t.Context()
		orgA, orgB := f.seedOrg(t, "acme"), f.seedOrg(t, "globex")
		require.NoError(t, f.projects.Save(ctx, newProject(t, orgA, project.DefaultSlug, uuid.Nil)))
		require.NoError(t, f.projects.Save(ctx, newProject(t, orgB, project.DefaultSlug, uuid.Nil)))
	})

	t.Run("public_id unique across orgs", func(t *testing.T) {
		f := newProjectFixture(t)
		ctx := t.Context()
		orgA, orgB := f.seedOrg(t, "acme"), f.seedOrg(t, "globex")
		a := newProject(t, orgA, "web", uuid.Nil)
		require.NoError(t, f.projects.Save(ctx, a))

		b := newProject(t, orgB, "api", uuid.Nil)
		b.PublicID = a.PublicID
		err := f.projects.Save(ctx, b)
		assert.True(t, project.IsAlreadyExistsError(err), "want AlreadyExistsError, got %T: %v", err, err)
	})

	t.Run("rename into a sibling's slug", func(t *testing.T) {
		f := newProjectFixture(t)
		ctx := t.Context()
		orgID := f.seedOrg(t, "acme")
		web, api := newProject(t, orgID, "web", uuid.Nil), newProject(t, orgID, "api", uuid.Nil)
		require.NoError(t, f.projects.Save(ctx, web))
		require.NoError(t, f.projects.Save(ctx, api))

		api.Slug = "web"
		err := f.projects.Save(ctx, api)
		assert.True(t, project.IsAlreadyExistsError(err), "want AlreadyExistsError, got %T: %v", err, err)
		got, err := f.projects.ByID(ctx, api.ID)
		require.NoError(t, err)
		assert.Equal(t, "api", got.Slug, "the failed update must not change the row")
	})
}

func TestProjectStore_UnknownOrgIsForeignKeyViolation(t *testing.T) {
	f := newProjectFixture(t)
	p := newProject(t, uuid.New(), "web", uuid.Nil)

	err := f.projects.Save(t.Context(), p)
	require.Error(t, err)
	assert.False(t, project.IsAlreadyExistsError(err), "a missing parent org must not look like a slug collision")
	assert.False(t, project.IsExpectedError(err))
	pgErr, ok := errors.AsType[*pgconn.PgError](err)
	require.True(t, ok, "want the raw *pgconn.PgError wrapped, got %T: %v", err, err)
	assert.Equal(t, "23503", pgErr.Code)
}

func TestProjectStore_DeletingOrgCascades(t *testing.T) {
	f := newProjectFixture(t)
	ctx := t.Context()
	orgA, orgB := f.seedOrg(t, "acme"), f.seedOrg(t, "globex")
	a1, a2 := newProject(t, orgA, "web", uuid.Nil), newProject(t, orgA, "api", uuid.Nil)
	b1 := newProject(t, orgB, "web", uuid.Nil)
	for _, p := range []*project.Project{a1, a2, b1} {
		require.NoError(t, f.projects.Save(ctx, p))
	}

	_, err := f.sqlDB.ExecContext(ctx, `DELETE FROM orgs WHERE id = $1`, orgA)
	require.NoError(t, err)

	for _, id := range []uuid.UUID{a1.ID, a2.ID} {
		_, err := f.projects.ByID(ctx, id)
		assert.True(t, project.IsNotFoundError(err), "project %s must be cascaded away, got %v", id, err)
	}
	got, err := f.projects.ByID(ctx, b1.ID)
	require.NoError(t, err, "another org's projects must survive")
	assert.Equal(t, orgB, got.OrgID)
}

func TestProjectStore_TenantScoping(t *testing.T) {
	f := newProjectFixture(t)
	ctx := t.Context()
	orgA, orgB := f.seedOrg(t, "acme"), f.seedOrg(t, "globex")
	require.NoError(t, f.projects.Save(ctx, newProject(t, orgA, "only-in-a", uuid.Nil)))
	require.NoError(t, f.projects.Save(ctx, newProject(t, orgA, "shared", uuid.Nil)))
	bShared := newProject(t, orgB, "shared", uuid.Nil)
	require.NoError(t, f.projects.Save(ctx, bShared))

	_, err := f.projects.BySlug(ctx, orgB, "only-in-a")
	nf, ok := errors.AsType[*project.NotFoundError](err)
	require.True(t, ok, "want NotFoundError, got %T: %v", err, err)
	assert.Equal(t, "only-in-a", nf.Slug)

	got, err := f.projects.BySlug(ctx, orgB, "shared")
	require.NoError(t, err)
	assert.Equal(t, bShared.ID, got.ID, "BySlug must resolve inside the given org")

	inA, err := f.projects.List(ctx, orgA)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"only-in-a", "shared"}, slugsOf(inA))
	for _, p := range inA {
		assert.Equal(t, orgA, p.OrgID)
	}

	none, err := f.projects.List(ctx, uuid.New())
	require.NoError(t, err)
	assert.Empty(t, none)

	id := uuid.New()
	_, err = f.projects.ByID(ctx, id)
	nf, ok = errors.AsType[*project.NotFoundError](err)
	require.True(t, ok, "want NotFoundError, got %T: %v", err, err)
	assert.Equal(t, id.String(), nf.ID)
}

func TestProjectStore_ListOrdersByCreatedAtAscending(t *testing.T) {
	f := newProjectFixture(t)
	ctx := t.Context()
	orgID := f.seedOrg(t, "acme")

	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	offsets := map[string]time.Duration{"charlie": 3 * time.Hour, "alpha": time.Hour, "bravo": 2 * time.Hour}
	for _, slug := range []string{"charlie", "alpha", "bravo"} {
		p := newProject(t, orgID, slug, uuid.Nil)
		p.CreatedAt = base.Add(offsets[slug])
		require.NoError(t, f.projects.Save(ctx, p))
	}

	got, err := f.projects.List(ctx, orgID)
	require.NoError(t, err)
	assert.Equal(t, []string{"alpha", "bravo", "charlie"}, slugsOf(got))
}

func TestProjectStore_HonorsUnitOfWork(t *testing.T) {
	errAbort := errors.New("abort")

	t.Run("rollback undoes org and project together", func(t *testing.T) {
		f := newProjectFixture(t)
		o, err := org.NewOrg("acme", "Acme", uuid.Nil)
		require.NoError(t, err)
		p := newProject(t, o.ID, project.DefaultSlug, uuid.Nil)

		err = db.RunInTx(t.Context(), f.pool, func(ctx context.Context) error {
			require.NoError(t, f.orgs.Save(ctx, o))
			require.NoError(t, f.projects.Save(ctx, p), "project insert sees the uncommitted org through the same tx")
			got, err := f.projects.BySlug(ctx, o.ID, project.DefaultSlug)
			require.NoError(t, err)
			assert.Equal(t, p.ID, got.ID)

			_, err = f.projects.ByID(t.Context(), p.ID)
			assert.True(t, project.IsNotFoundError(err), "uncommitted row must be invisible outside the tx")
			return errAbort
		})
		require.ErrorIs(t, err, errAbort)

		_, err = f.projects.ByID(t.Context(), p.ID)
		assert.True(t, project.IsNotFoundError(err), "rolled-back project must not persist, got %v", err)
		_, err = f.orgs.ByID(t.Context(), o.ID)
		assert.True(t, org.IsNotFoundError(err), "rolled-back org must not persist, got %v", err)
	})

	t.Run("commit persists", func(t *testing.T) {
		f := newProjectFixture(t)
		orgID := f.seedOrg(t, "acme")
		p := newProject(t, orgID, "web", uuid.Nil)
		require.NoError(t, db.RunInTx(t.Context(), f.pool, func(ctx context.Context) error {
			return f.projects.Save(ctx, p)
		}))
		got, err := f.projects.ByID(t.Context(), p.ID)
		require.NoError(t, err)
		assert.Equal(t, "web", got.Slug)
	})
}

func TestProjectStore_ConcurrentSameSlugExactlyOneWins(t *testing.T) {
	f := newProjectFixture(t)
	ctx := t.Context()
	orgID := f.seedOrg(t, "acme")
	const workers = 8

	start := make(chan struct{})
	errs := make([]error, workers)
	var wg sync.WaitGroup
	for i := range workers {
		p := newProject(t, orgID, "contested", uuid.Nil)
		wg.Go(func() {
			<-start
			errs[i] = f.projects.Save(ctx, p)
		})
	}
	close(start)
	wg.Wait()

	var ok, dup int
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case project.IsAlreadyExistsError(err):
			dup++
		default:
			t.Errorf("unexpected error: %T: %v", err, err)
		}
	}
	assert.Equal(t, 1, ok)
	assert.Equal(t, workers-1, dup)

	all, err := f.projects.List(ctx, orgID)
	require.NoError(t, err)
	assert.Len(t, all, 1)
}

func TestProjectStore_ByPublicID(t *testing.T) {
	f := newProjectFixture(t)
	p := newProject(t, f.seedOrg(t, "acme"), "web", uuid.Nil)
	require.NoError(t, f.projects.Save(t.Context(), p))

	got, err := f.projects.ByPublicID(t.Context(), p.PublicID)
	require.NoError(t, err)
	assert.Equal(t, p.ID, got.ID)
	for _, bad := range []string{"", "nope", p.ID.String()} {
		_, err = f.projects.ByPublicID(t.Context(), bad)
		assert.True(t, project.IsNotFoundError(err), "%q is not a public id", bad)
	}
}
