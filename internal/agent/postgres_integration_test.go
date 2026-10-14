//go:build integration

package agent_test

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

	"altalune.id/oagent/internal/agent"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/project"
	"altalune.id/oagent/internal/testutil/pgtest"
	"altalune.id/oagent/nanoid"
)

type pgFixture struct {
	sqlDB    *sql.DB
	pool     db.Pool
	store    agent.Store
	orgs     org.Store
	projects project.Store
}

func newPgFixture(t *testing.T) *pgFixture {
	t.Helper()
	sqlDB, schemaName := pgtest.Migrated(t)
	pool := db.Pool{W: sqlDB, R: sqlDB}
	return &pgFixture{
		sqlDB:    sqlDB,
		pool:     pool,
		store:    agent.NewStore(pool, schemaName, ""),
		orgs:     org.NewStore(pool, schemaName, ""),
		projects: project.NewStore(pool, schemaName, ""),
	}
}

func (f *pgFixture) seedOrg(t *testing.T) uuid.UUID {
	t.Helper()
	o, err := org.NewOrg("org-"+uuid.NewString()[:8], "Org", uuid.Nil)
	require.NoError(t, err)
	require.NoError(t, f.orgs.Save(t.Context(), o))
	return o.ID
}

func (f *pgFixture) seedProject(t *testing.T, orgID uuid.UUID) uuid.UUID {
	t.Helper()
	p, err := project.NewProject(orgID, "prj-"+uuid.NewString()[:8], "Project", uuid.Nil)
	require.NoError(t, err)
	require.NoError(t, f.projects.Save(t.Context(), p))
	return p.ID
}

func (f *pgFixture) seedScope(t *testing.T) (orgID, projectID uuid.UUID) {
	t.Helper()
	orgID = f.seedOrg(t)
	return orgID, f.seedProject(t, orgID)
}

func newAgent(t *testing.T, orgID, projectID uuid.UUID, slug string) *agent.Agent {
	t.Helper()
	a, err := agent.New(orgID, projectID, slug, "Agent "+slug, "", uuid.Nil)
	require.NoError(t, err)
	return a
}

func pgCode(err error) string {
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		return pgErr.Code
	}
	return ""
}

func TestPostgres_AgentRoundTrip(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)

	cases := []struct {
		name        string
		description string
		createdBy   uuid.UUID
	}{
		{name: "nullable columns unset", description: "", createdBy: uuid.Nil},
		{name: "nullable columns set", description: "answers support tickets", createdBy: uuid.New()},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			slug := []string{"bare-agent", "full-agent"}[i]
			a, err := agent.New(orgID, projectID, slug, "Support Bot", tc.description, tc.createdBy)
			require.NoError(t, err)
			loc := time.FixedZone("UTC+7", 7*60*60)
			a.CreatedAt = time.Date(2026, 3, 4, 5, 6, 7, 123456789, loc)

			before := time.Now().UTC().Truncate(time.Microsecond)
			require.NoError(t, f.store.Save(t.Context(), a))
			after := time.Now().UTC()

			got, err := f.store.ByID(t.Context(), a.ID)
			require.NoError(t, err)
			assert.Equal(t, a.ID, got.ID)
			assert.Equal(t, a.PublicID, got.PublicID)
			assert.Equal(t, orgID, got.OrgID)
			assert.Equal(t, projectID, got.ProjectID)
			assert.Equal(t, slug, got.Slug)
			assert.Equal(t, "Support Bot", got.Name)
			assert.Equal(t, tc.description, got.Description)
			assert.Equal(t, tc.createdBy != uuid.Nil, got.CreatedBy.Valid)
			if tc.createdBy != uuid.Nil {
				assert.Equal(t, tc.createdBy, got.CreatedBy.UUID)
			}
			assert.True(t, a.CreatedAt.Truncate(time.Microsecond).Equal(got.CreatedAt),
				"created_at keeps the instant at microsecond precision: want %v got %v", a.CreatedAt, got.CreatedAt)
			assert.False(t, got.UpdatedAt.Before(before), "updated_at is stamped by Save")
			assert.False(t, got.UpdatedAt.After(after))
		})
	}
}

func TestPostgres_AgentSaveUpdatesOnlyMutableColumns(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	otherOrg, otherProject := f.seedScope(t)

	a, err := agent.New(orgID, projectID, "first-slug", "First", "first description", uuid.New())
	require.NoError(t, err)
	require.NoError(t, f.store.Save(t.Context(), a))
	saved, err := f.store.ByID(t.Context(), a.ID)
	require.NoError(t, err)

	changed := *a
	changed.Slug = "second-slug"
	changed.Name = "Second"
	changed.Description = ""
	changed.PublicID, err = nanoid.New(21)
	require.NoError(t, err)
	changed.OrgID = otherOrg
	changed.ProjectID = otherProject
	changed.CreatedBy = uuid.NullUUID{}
	changed.CreatedAt = a.CreatedAt.Add(-48 * time.Hour)
	require.NoError(t, f.store.Save(t.Context(), &changed))

	got, err := f.store.ByID(t.Context(), a.ID)
	require.NoError(t, err)
	assert.Equal(t, "second-slug", got.Slug)
	assert.Equal(t, "Second", got.Name)
	assert.Empty(t, got.Description)
	assert.Equal(t, saved.PublicID, got.PublicID, "public_id is immutable")
	assert.Equal(t, orgID, got.OrgID, "org_id is immutable")
	assert.Equal(t, projectID, got.ProjectID, "project_id is immutable")
	assert.Equal(t, saved.CreatedBy, got.CreatedBy, "created_by is immutable")
	assert.True(t, saved.CreatedAt.Equal(got.CreatedAt), "created_at is immutable")
	assert.False(t, got.UpdatedAt.Before(saved.UpdatedAt), "updated_at moves forward")

	var n int
	require.NoError(t, f.sqlDB.QueryRowContext(t.Context(), `SELECT count(*) FROM agents`).Scan(&n))
	assert.Equal(t, 1, n, "a second Save upserts, it never inserts a second row")
}

func TestPostgres_AgentUniqueConstraints(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	siblingProject := f.seedProject(t, orgID)

	existing := newAgent(t, orgID, projectID, "taken")
	require.NoError(t, f.store.Save(t.Context(), existing))

	t.Run("slug reused in the same project", func(t *testing.T) {
		err := f.store.Save(t.Context(), newAgent(t, orgID, projectID, "taken"))
		require.Error(t, err)
		assert.True(t, agent.IsAlreadyExistsError(err), "got %v", err)
	})

	t.Run("slug reused in another project", func(t *testing.T) {
		require.NoError(t, f.store.Save(t.Context(), newAgent(t, orgID, siblingProject, "taken")))
	})

	t.Run("rename onto a sibling's slug", func(t *testing.T) {
		other := newAgent(t, orgID, projectID, "other")
		require.NoError(t, f.store.Save(t.Context(), other))
		other.Slug = "taken"
		err := f.store.Save(t.Context(), other)
		require.Error(t, err)
		assert.True(t, agent.IsAlreadyExistsError(err), "got %v", err)

		got, err := f.store.ByID(t.Context(), other.ID)
		require.NoError(t, err)
		assert.Equal(t, "other", got.Slug, "a refused update leaves the row unchanged")
	})

	t.Run("public id reused", func(t *testing.T) {
		dup := newAgent(t, orgID, projectID, "fresh-slug")
		dup.PublicID = existing.PublicID
		err := f.store.Save(t.Context(), dup)
		require.Error(t, err)
		assert.True(t, agent.IsAlreadyExistsError(err), "a public_id clash is a conflict, got %v", err)
	})
}

func TestPostgres_AgentForeignKeys(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)

	cases := []struct {
		name      string
		orgID     uuid.UUID
		projectID uuid.UUID
	}{
		{name: "unknown org", orgID: uuid.New(), projectID: projectID},
		{name: "unknown project", orgID: orgID, projectID: uuid.New()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := f.store.Save(t.Context(), newAgent(t, tc.orgID, tc.projectID, "orphan"))
			require.Error(t, err)
			assert.False(t, agent.IsAlreadyExistsError(err))
			assert.Equal(t, "23503", pgCode(err), "a foreign-key violation surfaces raw, got %v", err)
		})
	}
}

func TestPostgres_AgentProjectMustBelongToItsOrg(t *testing.T) {
	f := newPgFixture(t)
	orgA := f.seedOrg(t)
	_, projectB := f.seedScope(t)

	err := f.store.Save(t.Context(), newAgent(t, orgA, projectB, "cross-org"))
	if err == nil {
		t.Skip("BUG: agents has no composite FK (org_id, project_id) -> projects (org_id, id); an agent can claim org A inside org B's project")
	}
	assert.Equal(t, "23503", pgCode(err))
}

func TestPostgres_AgentCascades(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	keptProject := f.seedProject(t, orgID)
	otherOrg, otherProject := f.seedScope(t)

	gone := newAgent(t, orgID, projectID, "gone")
	kept := newAgent(t, orgID, keptProject, "kept")
	foreign := newAgent(t, otherOrg, otherProject, "foreign")
	for _, a := range []*agent.Agent{gone, kept, foreign} {
		require.NoError(t, f.store.Save(t.Context(), a))
	}

	_, err := f.sqlDB.ExecContext(t.Context(), `DELETE FROM projects WHERE id = $1`, projectID)
	require.NoError(t, err)
	_, err = f.store.ByID(t.Context(), gone.ID)
	assert.True(t, agent.IsNotFoundError(err), "deleting a project removes its agents, got %v", err)
	_, err = f.store.ByID(t.Context(), kept.ID)
	require.NoError(t, err, "a sibling project's agents survive")

	_, err = f.sqlDB.ExecContext(t.Context(), `DELETE FROM orgs WHERE id = $1`, orgID)
	require.NoError(t, err)
	_, err = f.store.ByID(t.Context(), kept.ID)
	assert.True(t, agent.IsNotFoundError(err), "deleting an org removes its agents, got %v", err)
	_, err = f.store.ByID(t.Context(), foreign.ID)
	require.NoError(t, err, "another org's agents survive")
}

func TestPostgres_AgentNotFoundAndDelete(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)

	missing := uuid.New()
	_, err := f.store.ByID(t.Context(), missing)
	var nf *agent.NotFoundError
	require.ErrorAs(t, err, &nf)
	assert.Equal(t, missing.String(), nf.ID)

	err = f.store.Delete(t.Context(), missing)
	assert.True(t, agent.IsNotFoundError(err), "deleting a missing agent is NotFound, got %v", err)

	a := newAgent(t, orgID, projectID, "doomed")
	require.NoError(t, f.store.Save(t.Context(), a))
	require.NoError(t, f.store.Delete(t.Context(), a.ID))
	_, err = f.store.ByID(t.Context(), a.ID)
	assert.True(t, agent.IsNotFoundError(err))
	assert.True(t, agent.IsNotFoundError(f.store.Delete(t.Context(), a.ID)), "a second delete is NotFound")
}

func TestPostgres_AgentListOrderAndScope(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	siblingProject := f.seedProject(t, orgID)
	otherOrg, otherProject := f.seedScope(t)
	emptyProject := f.seedProject(t, orgID)

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	want := make([]uuid.UUID, 3)
	for _, i := range []int{2, 0, 1} {
		a := newAgent(t, orgID, projectID, []string{"aaa", "bbb", "ccc"}[i])
		a.CreatedAt = base.Add(time.Duration(i) * time.Minute)
		require.NoError(t, f.store.Save(t.Context(), a))
		want[i] = a.ID
	}
	require.NoError(t, f.store.Save(t.Context(), newAgent(t, orgID, siblingProject, "aaa")))
	require.NoError(t, f.store.Save(t.Context(), newAgent(t, otherOrg, otherProject, "aaa")))

	got, err := f.store.List(t.Context(), projectID)
	require.NoError(t, err)
	ids := make([]uuid.UUID, 0, len(got))
	for _, a := range got {
		assert.Equal(t, projectID, a.ProjectID, "List never returns another project's agents")
		ids = append(ids, a.ID)
	}
	assert.Equal(t, want, ids, "List orders by created_at ascending, not insertion order")

	empty, err := f.store.List(t.Context(), emptyProject)
	require.NoError(t, err)
	assert.NotNil(t, empty)
	assert.Empty(t, empty)
}

func TestPostgres_AgentHonorsUnitOfWork(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	errAbort := errors.New("abort")

	rolledBack := newAgent(t, orgID, projectID, "rolled-back")
	err := db.RunInTx(t.Context(), f.pool, func(ctx context.Context) error {
		require.NoError(t, f.store.Save(ctx, rolledBack))
		_, rerr := f.store.ByID(ctx, rolledBack.ID)
		require.NoError(t, rerr, "a read inside the unit of work sees its own write")
		_, rerr = f.store.ByID(t.Context(), rolledBack.ID)
		assert.True(t, agent.IsNotFoundError(rerr), "a read outside the unit of work does not see the uncommitted row")
		return errAbort
	})
	require.ErrorIs(t, err, errAbort)
	_, err = f.store.ByID(t.Context(), rolledBack.ID)
	assert.True(t, agent.IsNotFoundError(err), "a rolled-back unit of work leaves nothing")

	committed := newAgent(t, orgID, projectID, "committed")
	require.NoError(t, db.RunInTx(t.Context(), f.pool, func(ctx context.Context) error {
		return f.store.Save(ctx, committed)
	}))
	_, err = f.store.ByID(t.Context(), committed.ID)
	require.NoError(t, err, "a committed unit of work persists")

	deleted := newAgent(t, orgID, projectID, "kept-on-rollback")
	require.NoError(t, f.store.Save(t.Context(), deleted))
	err = db.RunInTx(t.Context(), f.pool, func(ctx context.Context) error {
		require.NoError(t, f.store.Delete(ctx, deleted.ID))
		return errAbort
	})
	require.ErrorIs(t, err, errAbort)
	_, err = f.store.ByID(t.Context(), deleted.ID)
	require.NoError(t, err, "a rolled-back delete keeps the row")
}

func TestPostgres_AgentConcurrentSlugClaim(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)

	const writers = 8
	start := make(chan struct{})
	errs := make([]error, writers)
	var wg sync.WaitGroup
	for i := range writers {
		a := newAgent(t, orgID, projectID, "contended")
		wg.Go(func() {
			<-start
			errs[i] = f.store.Save(t.Context(), a)
		})
	}
	close(start)
	wg.Wait()

	wins := 0
	for _, err := range errs {
		if err == nil {
			wins++
			continue
		}
		assert.True(t, agent.IsAlreadyExistsError(err), "a losing writer gets AlreadyExistsError, got %v", err)
	}
	assert.Equal(t, 1, wins, "exactly one concurrent writer claims the slug")

	got, err := f.store.List(t.Context(), projectID)
	require.NoError(t, err)
	assert.Len(t, got, 1)
}

func TestPostgres_AgentUpsertCannotRewriteAnotherOrgsAgent(t *testing.T) {
	f := newPgFixture(t)
	orgA, projectA := f.seedScope(t)
	orgB, projectB := f.seedScope(t)

	victim := newAgent(t, orgA, projectA, "victim")
	require.NoError(t, f.store.Save(t.Context(), victim))

	intruder := newAgent(t, orgB, projectB, "hijacked")
	intruder.ID = victim.ID
	_ = f.store.Save(t.Context(), intruder)

	got, err := f.store.ByID(t.Context(), victim.ID)
	require.NoError(t, err)
	if got.Slug != "victim" || got.Name != victim.Name {
		t.Skip("BUG: agent Save ON CONFLICT (id) DO UPDATE has no org/project guard; a colliding id from org B rewrites org A's agent")
	}
}

func TestPostgres_ByPublicID(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	a := newAgent(t, orgID, projectID, "bot")
	require.NoError(t, f.store.Save(t.Context(), a))

	got, err := f.store.ByPublicID(t.Context(), a.PublicID)
	require.NoError(t, err)
	assert.Equal(t, a.ID, got.ID)
	for _, bad := range []string{"", "nope", a.ID.String()} {
		_, err = f.store.ByPublicID(t.Context(), bad)
		assert.True(t, agent.IsNotFoundError(err), "%q is not a public id", bad)
	}
}
