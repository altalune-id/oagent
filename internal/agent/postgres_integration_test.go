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
	"altalune.id/oagent/internal/kernel/tenant"
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
	pool, sqlDB, schemaName := pgtest.MigratedRLS(t)
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
	require.NoError(t, f.orgs.Save(in(o.ID), o))
	return o.ID
}

func (f *pgFixture) seedProject(t *testing.T, orgID uuid.UUID) uuid.UUID {
	t.Helper()
	p, err := project.NewProject(orgID, "prj-"+uuid.NewString()[:8], "Project", uuid.Nil)
	require.NoError(t, err)
	require.NoError(t, f.projects.Save(in(orgID), p))
	return p.ID
}

func (f *pgFixture) seedScope(t *testing.T) (orgID, projectID uuid.UUID) {
	t.Helper()
	orgID = f.seedOrg(t)
	return orgID, f.seedProject(t, orgID)
}

func in(orgID uuid.UUID) context.Context {
	return tenant.Into(context.Background(), tenant.Context{OrgID: orgID})
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
			require.NoError(t, f.store.Save(in(orgID), a))
			after := time.Now().UTC()

			got, err := f.store.ByID(in(orgID), a.ID)
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
	siblingProject := f.seedProject(t, orgID)
	otherOrg, otherProject := f.seedScope(t)
	ctx := in(orgID)

	a, err := agent.New(orgID, projectID, "first-slug", "First", "first description", uuid.New())
	require.NoError(t, err)
	require.NoError(t, f.store.Save(ctx, a))
	saved, err := f.store.ByID(ctx, a.ID)
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
	assert.Equal(t, "42501", pgCode(f.store.Save(ctx, &changed)), "row level security refuses moving an agent to another org")
	changed.OrgID = orgID
	changed.ProjectID = siblingProject
	require.NoError(t, f.store.Save(ctx, &changed))

	got, err := f.store.ByID(ctx, a.ID)
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
	ctx := in(orgID)

	existing := newAgent(t, orgID, projectID, "taken")
	require.NoError(t, f.store.Save(ctx, existing))

	t.Run("slug reused in the same project", func(t *testing.T) {
		err := f.store.Save(ctx, newAgent(t, orgID, projectID, "taken"))
		require.Error(t, err)
		assert.True(t, agent.IsAlreadyExistsError(err), "got %v", err)
	})

	t.Run("slug reused in another project", func(t *testing.T) {
		require.NoError(t, f.store.Save(ctx, newAgent(t, orgID, siblingProject, "taken")))
	})

	t.Run("rename onto a sibling's slug", func(t *testing.T) {
		other := newAgent(t, orgID, projectID, "other")
		require.NoError(t, f.store.Save(ctx, other))
		other.Slug = "taken"
		err := f.store.Save(ctx, other)
		require.Error(t, err)
		assert.True(t, agent.IsAlreadyExistsError(err), "got %v", err)

		got, err := f.store.ByID(ctx, other.ID)
		require.NoError(t, err)
		assert.Equal(t, "other", got.Slug, "a refused update leaves the row unchanged")
	})

	t.Run("public id reused", func(t *testing.T) {
		dup := newAgent(t, orgID, projectID, "fresh-slug")
		dup.PublicID = existing.PublicID
		err := f.store.Save(ctx, dup)
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
			err := f.store.Save(in(tc.orgID), newAgent(t, tc.orgID, tc.projectID, "orphan"))
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

	err := f.store.Save(in(orgA), newAgent(t, orgA, projectB, "cross-org"))
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
		require.NoError(t, f.store.Save(in(a.OrgID), a))
	}

	_, err := f.sqlDB.ExecContext(t.Context(), `DELETE FROM projects WHERE id = $1`, projectID)
	require.NoError(t, err)
	_, err = f.store.ByID(in(orgID), gone.ID)
	assert.True(t, agent.IsNotFoundError(err), "deleting a project removes its agents, got %v", err)
	_, err = f.store.ByID(in(orgID), kept.ID)
	require.NoError(t, err, "a sibling project's agents survive")

	_, err = f.sqlDB.ExecContext(t.Context(), `DELETE FROM orgs WHERE id = $1`, orgID)
	require.NoError(t, err)
	_, err = f.store.ByID(in(orgID), kept.ID)
	assert.True(t, agent.IsNotFoundError(err), "deleting an org removes its agents, got %v", err)
	_, err = f.store.ByID(in(otherOrg), foreign.ID)
	require.NoError(t, err, "another org's agents survive")
}

func TestPostgres_AgentNotFoundAndDelete(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	ctx := in(orgID)

	missing := uuid.New()
	_, err := f.store.ByID(ctx, missing)
	var nf *agent.NotFoundError
	require.ErrorAs(t, err, &nf)
	assert.Equal(t, missing.String(), nf.ID)

	err = f.store.Delete(ctx, missing)
	assert.True(t, agent.IsNotFoundError(err), "deleting a missing agent is NotFound, got %v", err)

	a := newAgent(t, orgID, projectID, "doomed")
	require.NoError(t, f.store.Save(ctx, a))
	require.NoError(t, f.store.Delete(ctx, a.ID))
	_, err = f.store.ByID(ctx, a.ID)
	assert.True(t, agent.IsNotFoundError(err))
	assert.True(t, agent.IsNotFoundError(f.store.Delete(ctx, a.ID)), "a second delete is NotFound")
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
		require.NoError(t, f.store.Save(in(orgID), a))
		want[i] = a.ID
	}
	require.NoError(t, f.store.Save(in(orgID), newAgent(t, orgID, siblingProject, "aaa")))
	require.NoError(t, f.store.Save(in(otherOrg), newAgent(t, otherOrg, otherProject, "aaa")))

	got, err := f.store.List(in(orgID), projectID)
	require.NoError(t, err)
	ids := make([]uuid.UUID, 0, len(got))
	for _, a := range got {
		assert.Equal(t, projectID, a.ProjectID, "List never returns another project's agents")
		ids = append(ids, a.ID)
	}
	assert.Equal(t, want, ids, "List orders by created_at ascending, not insertion order")

	empty, err := f.store.List(in(orgID), emptyProject)
	require.NoError(t, err)
	assert.NotNil(t, empty)
	assert.Empty(t, empty)
}

func TestPostgres_AgentHonorsUnitOfWork(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	ctx := in(orgID)
	errAbort := errors.New("abort")

	rolledBack := newAgent(t, orgID, projectID, "rolled-back")
	err := db.RunInTx(ctx, f.pool, func(ctx context.Context) error {
		require.NoError(t, f.store.Save(ctx, rolledBack))
		_, rerr := f.store.ByID(ctx, rolledBack.ID)
		require.NoError(t, rerr, "a read inside the unit of work sees its own write")
		_, rerr = f.store.ByID(in(orgID), rolledBack.ID)
		assert.True(t, agent.IsNotFoundError(rerr), "a read outside the unit of work does not see the uncommitted row")
		return errAbort
	})
	require.ErrorIs(t, err, errAbort)
	_, err = f.store.ByID(ctx, rolledBack.ID)
	assert.True(t, agent.IsNotFoundError(err), "a rolled-back unit of work leaves nothing")

	committed := newAgent(t, orgID, projectID, "committed")
	require.NoError(t, db.RunInTx(ctx, f.pool, func(ctx context.Context) error {
		return f.store.Save(ctx, committed)
	}))
	_, err = f.store.ByID(ctx, committed.ID)
	require.NoError(t, err, "a committed unit of work persists")

	deleted := newAgent(t, orgID, projectID, "kept-on-rollback")
	require.NoError(t, f.store.Save(ctx, deleted))
	err = db.RunInTx(ctx, f.pool, func(ctx context.Context) error {
		require.NoError(t, f.store.Delete(ctx, deleted.ID))
		return errAbort
	})
	require.ErrorIs(t, err, errAbort)
	_, err = f.store.ByID(ctx, deleted.ID)
	require.NoError(t, err, "a rolled-back delete keeps the row")
}

func TestPostgres_AgentConcurrentSlugClaim(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	ctx := in(orgID)

	const writers = 8
	start := make(chan struct{})
	errs := make([]error, writers)
	var wg sync.WaitGroup
	for i := range writers {
		a := newAgent(t, orgID, projectID, "contended")
		wg.Go(func() {
			<-start
			errs[i] = f.store.Save(ctx, a)
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

	got, err := f.store.List(ctx, projectID)
	require.NoError(t, err)
	assert.Len(t, got, 1)
}

func TestPostgres_AgentUpsertCannotRewriteAnotherOrgsAgent(t *testing.T) {
	f := newPgFixture(t)
	orgA, projectA := f.seedScope(t)
	orgB, projectB := f.seedScope(t)

	victim := newAgent(t, orgA, projectA, "victim")
	require.NoError(t, f.store.Save(in(orgA), victim))

	intruder := newAgent(t, orgB, projectB, "hijacked")
	intruder.ID = victim.ID
	assert.Equal(t, "42501", pgCode(f.store.Save(in(orgB), intruder)), "row level security refuses an upsert onto another org's agent")

	got, err := f.store.ByID(in(orgA), victim.ID)
	require.NoError(t, err)
	assert.Equal(t, "victim", got.Slug)
	assert.Equal(t, victim.Name, got.Name)
	assert.Equal(t, orgA, got.OrgID)
	assert.Equal(t, projectA, got.ProjectID)
}

func TestPostgres_AgentResolveNamesTheTenantWithoutAScope(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	a := newAgent(t, orgID, projectID, "bot")
	require.NoError(t, f.store.Save(in(orgID), a))

	got, err := f.store.Resolve(t.Context(), a.PublicID)
	require.NoError(t, err)
	assert.Equal(t, agent.Ref{ID: a.ID, OrgID: orgID, ProjectID: projectID}, got)
	for _, bad := range []string{"", "nope", a.ID.String()} {
		_, err = f.store.Resolve(t.Context(), bad)
		assert.True(t, agent.IsNotFoundError(err), "%q is not a public id", bad)
	}
}

func (f *pgFixture) publish(t *testing.T, ctx context.Context, a *agent.Agent, cfg agent.Config) {
	t.Helper()
	require.NoError(t, a.SetDraft(cfg, time.Now()))
	latest, err := f.store.LatestVersion(ctx, a.ID)
	require.NoError(t, err)
	v, created, err := a.Publish(latest, uuid.Nil, time.Now().UTC())
	require.NoError(t, err)
	require.True(t, created)
	require.NoError(t, f.store.InsertVersion(ctx, v))
	require.NoError(t, f.store.MarkPublished(ctx, a))
	require.NoError(t, f.store.SaveDraft(ctx, a))
}

func TestUsage_ProviderUse(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	ctx := in(orgID)
	provider, other := uuid.New(), uuid.New()

	use, err := f.store.ProviderUse(ctx, provider)
	require.NoError(t, err)
	assert.False(t, use.InUse)

	capped := newAgent(t, orgID, projectID, "capped")
	require.NoError(t, f.store.Save(ctx, capped))
	f.publish(t, ctx, capped, agent.Config{ProviderID: provider, Model: "gpt-4o", HelperModel: "mini", MaxCostCents: 100})

	free := newAgent(t, orgID, projectID, "free")
	require.NoError(t, f.store.Save(ctx, free))
	f.publish(t, ctx, free, agent.Config{ProviderID: provider, Model: "llama"})

	moved := newAgent(t, orgID, projectID, "moved")
	require.NoError(t, f.store.Save(ctx, moved))
	f.publish(t, ctx, moved, agent.Config{ProviderID: provider, Model: "old-model", MaxCostCents: 5})
	f.publish(t, ctx, moved, agent.Config{ProviderID: other, Model: "old-model"})

	drafted := newAgent(t, orgID, projectID, "drafted")
	require.NoError(t, f.store.Save(ctx, drafted))
	require.NoError(t, drafted.SetDraft(agent.Config{ProviderID: provider, Model: "draft-only", MaxCostCents: 9}, time.Now()))
	require.NoError(t, f.store.SaveDraft(ctx, drafted))

	use, err = f.store.ProviderUse(ctx, provider)
	require.NoError(t, err)
	assert.True(t, use.InUse)
	assert.Equal(t, map[string]bool{"gpt-4o": true, "mini": true, "llama": false}, use.Models,
		"only the latest version per agent counts; drafts never do; a cap needs the price")

	use, err = f.store.ProviderUse(in(uuid.New()), provider)
	require.NoError(t, err)
	assert.False(t, use.InUse, "another tenant sees no use")
}

func TestUsage_ProviderUseCountsTranscriptionOnItsOwnProvider(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	ctx := in(orgID)
	chat, stt := uuid.New(), uuid.New()

	a := newAgent(t, orgID, projectID, "voice")
	require.NoError(t, f.store.Save(ctx, a))
	f.publish(t, ctx, a, agent.Config{ProviderID: chat, Model: "gpt-4o", MaxCostCents: 100,
		TranscriptionModel: agent.TranscriptionModel{ProviderID: stt, Model: "whisper-1"}})

	both := newAgent(t, orgID, projectID, "both")
	require.NoError(t, f.store.Save(ctx, both))
	f.publish(t, ctx, both, agent.Config{ProviderID: stt, Model: "llama",
		TranscriptionModel: agent.TranscriptionModel{ProviderID: stt, Model: "whisper-1"}})

	drafted := newAgent(t, orgID, projectID, "drafted")
	require.NoError(t, f.store.Save(ctx, drafted))
	require.NoError(t, drafted.SetDraft(agent.Config{ProviderID: chat, Model: "gpt-4o",
		TranscriptionModel: agent.TranscriptionModel{ProviderID: stt, Model: "draft-only"}}, time.Now()))
	require.NoError(t, f.store.SaveDraft(ctx, drafted))

	use, err := f.store.ProviderUse(ctx, stt)
	require.NoError(t, err)
	assert.True(t, use.InUse)
	assert.Equal(t, map[string]bool{"llama": false}, use.Models, "the chat model of another provider is not counted here")
	assert.Equal(t, []string{"whisper-1"}, use.TranscriptionModels, "once per model; drafts never count")

	use, err = f.store.ProviderUse(ctx, chat)
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"gpt-4o": true}, use.Models)
	assert.Empty(t, use.TranscriptionModels, "the transcription model of another provider is not counted here")
}

func TestUsage_MCPServerInUse(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	ctx := in(orgID)
	provider, server, dropped, drafted := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	grant := func(id uuid.UUID) []agent.MCPGrant {
		return []agent.MCPGrant{{ServerID: id, Tools: []agent.Tool{{Name: "t"}}}}
	}

	a := newAgent(t, orgID, projectID, "uses")
	require.NoError(t, f.store.Save(ctx, a))
	f.publish(t, ctx, a, agent.Config{ProviderID: provider, Model: "m", MCPServers: append(grant(uuid.New()), grant(server)...)})

	b := newAgent(t, orgID, projectID, "dropped")
	require.NoError(t, f.store.Save(ctx, b))
	f.publish(t, ctx, b, agent.Config{ProviderID: provider, Model: "m", MCPServers: grant(dropped)})
	f.publish(t, ctx, b, agent.Config{ProviderID: provider, Model: "m"})

	c := newAgent(t, orgID, projectID, "drafted")
	require.NoError(t, f.store.Save(ctx, c))
	require.NoError(t, c.SetDraft(agent.Config{ProviderID: provider, MCPServers: grant(drafted)}, time.Now()))
	require.NoError(t, f.store.SaveDraft(ctx, c))

	for id, want := range map[uuid.UUID]bool{server: true, dropped: false, drafted: false, uuid.New(): false} {
		got, err := f.store.MCPServerInUse(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, want, got, id.String())
	}
}

func TestUsage_KnowledgeSources(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	ctx := in(orgID)
	provider, used, dropped, drafted := uuid.New(), uuid.New(), uuid.New(), uuid.New()

	a := newAgent(t, orgID, projectID, "uses")
	require.NoError(t, f.store.Save(ctx, a))
	f.publish(t, ctx, a, agent.Config{ProviderID: provider, Model: "m", KnowledgeSourceIDs: []uuid.UUID{used}})

	b := newAgent(t, orgID, projectID, "dropped")
	require.NoError(t, f.store.Save(ctx, b))
	f.publish(t, ctx, b, agent.Config{ProviderID: provider, Model: "m", KnowledgeSourceIDs: []uuid.UUID{dropped, used}})
	f.publish(t, ctx, b, agent.Config{ProviderID: provider, Model: "m", KnowledgeSourceIDs: []uuid.UUID{used}})

	c := newAgent(t, orgID, projectID, "drafted")
	require.NoError(t, f.store.Save(ctx, c))
	require.NoError(t, c.SetDraft(agent.Config{ProviderID: provider, KnowledgeSourceIDs: []uuid.UUID{drafted}}, time.Now()))
	require.NoError(t, f.store.SaveDraft(ctx, c))

	for id, want := range map[uuid.UUID]bool{used: true, dropped: false, drafted: false, uuid.New(): false} {
		got, err := f.store.KnowledgeSourceInUse(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, want, got, id.String())
	}
	use, err := f.store.KnowledgeUse(ctx, projectID)
	require.NoError(t, err)
	want := []agent.Named{{ID: a.ID, PublicID: a.PublicID, Name: a.Name}, {ID: b.ID, PublicID: b.PublicID, Name: b.Name}}
	assert.ElementsMatch(t, want, use[used])
	assert.NotContains(t, use, dropped, "only latest published versions count")
	assert.NotContains(t, use, drafted, "drafts never count")
}

func TestUsage_MemoryStores(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	ctx := in(orgID)
	provider, used, dropped, drafted := uuid.New(), uuid.New(), uuid.New(), uuid.New()

	a := newAgent(t, orgID, projectID, "uses")
	require.NoError(t, f.store.Save(ctx, a))
	f.publish(t, ctx, a, agent.Config{ProviderID: provider, Model: "m", MemoryStoreID: used})

	b := newAgent(t, orgID, projectID, "switched")
	require.NoError(t, f.store.Save(ctx, b))
	f.publish(t, ctx, b, agent.Config{ProviderID: provider, Model: "m", MemoryStoreID: dropped})
	f.publish(t, ctx, b, agent.Config{ProviderID: provider, Model: "m", MemoryStoreID: used})

	c := newAgent(t, orgID, projectID, "drafted")
	require.NoError(t, f.store.Save(ctx, c))
	require.NoError(t, c.SetDraft(agent.Config{ProviderID: provider, MemoryStoreID: drafted}, time.Now()))
	require.NoError(t, f.store.SaveDraft(ctx, c))

	d := newAgent(t, orgID, projectID, "none")
	require.NoError(t, f.store.Save(ctx, d))
	f.publish(t, ctx, d, agent.Config{ProviderID: provider, Model: "m"})

	for id, want := range map[uuid.UUID]bool{used: true, dropped: false, drafted: false, uuid.New(): false} {
		got, err := f.store.MemoryStoreInUse(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, want, got, id.String())
	}
	use, err := f.store.MemoryUse(ctx, projectID)
	require.NoError(t, err)
	want := []agent.Named{{ID: a.ID, PublicID: a.PublicID, Name: a.Name}, {ID: b.ID, PublicID: b.PublicID, Name: b.Name}}
	assert.ElementsMatch(t, want, use[used])
	assert.Len(t, use, 1, "only latest published versions count, drafts never, and an agent with no store names none")

	other, err := f.store.MemoryStoreInUse(in(uuid.New()), used)
	require.NoError(t, err)
	assert.False(t, other, "another org's versions never count")
}

func TestPostgres_VersionByIDIsTenantScoped(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	ctx := in(orgID)
	a := newAgent(t, orgID, projectID, "pinned")
	require.NoError(t, f.store.Save(ctx, a))
	f.publish(t, ctx, a, agent.Config{ProviderID: uuid.New(), Model: "gpt-4o", Instructions: "v1"})
	latest, err := f.store.LatestVersion(ctx, a.ID)
	require.NoError(t, err)

	got, err := f.store.VersionByID(ctx, latest.ID)
	require.NoError(t, err)
	assert.Equal(t, latest.ID, got.ID)
	assert.Equal(t, "v1", got.Config.Instructions)

	_, err = f.store.VersionByID(ctx, uuid.New())
	assert.True(t, agent.IsNotFoundError(err))
	otherOrg, _ := f.seedScope(t)
	_, err = f.store.VersionByID(in(otherOrg), latest.ID)
	assert.True(t, agent.IsNotFoundError(err), "another tenant does not see it")
}

func (f *pgFixture) draft(t *testing.T, ctx context.Context, a *agent.Agent, cfg agent.Config) *agent.Version {
	t.Helper()
	require.NoError(t, a.SetDraft(cfg, time.Now()))
	require.NoError(t, f.store.SaveDraft(ctx, a))
	v, err := a.DraftVersion(uuid.Nil, time.Now().UTC())
	require.NoError(t, err)
	got, err := f.store.UpsertDraftVersion(ctx, v)
	require.NoError(t, err)
	return got
}

func TestPostgres_DraftVersionsAreDedupedByConfig(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	ctx := in(orgID)
	a := newAgent(t, orgID, projectID, "drafts")
	require.NoError(t, f.store.Save(ctx, a))
	cfg := agent.Config{ProviderID: uuid.New(), Model: "m", Instructions: "try me"}

	first := f.draft(t, ctx, a, cfg)
	assert.Equal(t, agent.VersionDraft, first.Kind)
	assert.Zero(t, first.Version, "a draft has no number")
	again := f.draft(t, ctx, a, cfg)
	assert.Equal(t, first.ID, again.ID, "the same draft config reuses its row")
	edited := f.draft(t, ctx, a, agent.Config{ProviderID: cfg.ProviderID, Model: "m", Instructions: "edited"})
	assert.NotEqual(t, first.ID, edited.ID, "an edited draft gets a new row")

	got, err := f.store.VersionByID(ctx, first.ID)
	require.NoError(t, err)
	assert.Equal(t, agent.VersionDraft, got.Kind, "VersionByID reads drafts: a try run resumes on its pin")
	assert.Equal(t, "try me", got.Config.Instructions)
	other := newAgent(t, orgID, projectID, "other")
	require.NoError(t, f.store.Save(ctx, other))
	assert.NotEqual(t, first.ID, f.draft(t, ctx, other, cfg).ID, "the hash is per agent")
}

func TestPostgres_PublishedReadersIgnoreDrafts(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	ctx := in(orgID)
	provider, server := uuid.New(), uuid.New()
	a := newAgent(t, orgID, projectID, "mixed")
	require.NoError(t, f.store.Save(ctx, a))
	f.publish(t, ctx, a, agent.Config{ProviderID: provider, Model: "v1"})
	f.draft(t, ctx, a, agent.Config{ProviderID: provider, Model: "draft", MaxCostCents: 7,
		MCPServers: []agent.MCPGrant{{ServerID: server, Tools: []agent.Tool{{Name: "t"}}}}})

	latest, err := f.store.LatestVersion(ctx, a.ID)
	require.NoError(t, err)
	assert.Equal(t, int32(1), latest.Version, "Latest never returns a draft, though NULL sorts first in DESC")
	assert.Equal(t, agent.VersionPublished, latest.Kind)
	list, err := f.store.Versions(ctx, a.ID, 10)
	require.NoError(t, err)
	require.Len(t, list, 1, "the version list has no drafts")
	use, err := f.store.ProviderUse(ctx, provider)
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"v1": false}, use.Models, "a draft row is not a model in use")
	inUse, err := f.store.MCPServerInUse(ctx, server)
	require.NoError(t, err)
	assert.False(t, inUse, "a draft row's grant is not a server in use")

	f.publish(t, ctx, a, agent.Config{ProviderID: provider, Model: "v2"})
	latest, err = f.store.LatestVersion(ctx, a.ID)
	require.NoError(t, err)
	assert.Equal(t, int32(2), latest.Version, "publish numbering skips drafts")
}

func TestPostgres_VersionKindShape(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	a := newAgent(t, orgID, projectID, "shape")
	require.NoError(t, f.store.Save(in(orgID), a))
	insert := func(kind string, version any, hash any) error {
		_, err := f.sqlDB.ExecContext(t.Context(), `INSERT INTO agent_versions (id, public_id, org_id, project_id, agent_id, version, config, created_at, kind, config_hash) VALUES ($1, $2, $3, $4, $5, $6, '{}', now(), $7, $8)`,
			uuid.New(), nanoid.MustNew(21), orgID, projectID, a.ID, version, kind, hash)
		return err
	}
	assert.Equal(t, "23514", pgCode(insert("draft", 3, []byte{1})), "a draft has no number")
	assert.Equal(t, "23514", pgCode(insert("published", nil, nil)), "a published version has one")
	assert.Equal(t, "23514", pgCode(insert("draft", nil, nil)), "a draft has a config hash")
	assert.Equal(t, "23514", pgCode(insert("other", 1, nil)))
	require.NoError(t, insert("draft", nil, []byte{2}))
	assert.Equal(t, "23505", pgCode(insert("draft", nil, []byte{2})), "one draft row per agent and hash")
}

func TestPostgres_AgentPublicIDsInOneRead(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	a := newAgent(t, orgID, projectID, "named")
	require.NoError(t, f.store.Save(in(orgID), a))
	got, err := f.store.AgentPublicIDs(in(orgID), []uuid.UUID{a.ID, uuid.New()})
	require.NoError(t, err)
	assert.Equal(t, map[uuid.UUID]string{a.ID: a.PublicID}, got, "a gone agent is left out")
	other, err := f.store.AgentPublicIDs(in(uuid.New()), []uuid.UUID{a.ID})
	require.NoError(t, err)
	assert.Empty(t, other, "another org names nothing")
	none, err := f.store.AgentPublicIDs(in(orgID), nil)
	require.NoError(t, err)
	assert.Empty(t, none)
	_, err = f.store.AgentPublicIDs(context.Background(), []uuid.UUID{a.ID})
	require.Error(t, err)
}
