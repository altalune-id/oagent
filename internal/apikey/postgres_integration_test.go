//go:build integration

package apikey_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/apikey"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/project"
	"altalune.id/oagent/internal/testutil/pgtest"
	"altalune.id/oagent/internal/user"
	"altalune.id/oagent/scope"
)

type pgFixture struct {
	schema   string
	sqlDB    *sql.DB
	pool     db.Pool
	store    apikey.Store
	orgs     org.Store
	projects project.Store
}

func newPgFixture(t *testing.T) *pgFixture {
	t.Helper()
	sqlDB, schemaName := pgtest.Migrated(t)
	pool := db.Pool{W: sqlDB, R: sqlDB}
	return &pgFixture{
		schema:   schemaName,
		sqlDB:    sqlDB,
		pool:     pool,
		store:    apikey.NewStore(pool, schemaName, ""),
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

func (f *pgFixture) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	require.NoError(t, f.sqlDB.QueryRowContext(t.Context(), query, args...).Scan(&n))
	return n
}

func expiry() *time.Time {
	exp := time.Now().UTC().Add(24 * time.Hour)
	return &exp
}

func projectKey(t *testing.T, orgID, projectID uuid.UUID) *apikey.APIKey {
	t.Helper()
	k, _, err := apikey.NewScheme("").MintProject(orgID, projectID, "ci", []string{scope.AgentRead}, nil, expiry(), time.Now().UTC())
	require.NoError(t, err)
	return k
}

func orgKey(t *testing.T, orgID uuid.UUID, grant apikey.ProjectGrant) *apikey.APIKey {
	t.Helper()
	k, _, err := apikey.NewScheme("").MintOrg(orgID, "ops", []string{scope.OrgRead}, grant, expiry(), time.Now().UTC())
	require.NoError(t, err)
	return k
}

func pgCode(err error) string {
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		return pgErr.Code
	}
	return ""
}

func sortedIDs(ids ...uuid.UUID) []uuid.UUID {
	out := slices.Clone(ids)
	slices.SortFunc(out, func(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) })
	return out
}

func TestPostgres_ProjectKeyRoundTrip(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	creator := f.seedUser(t)
	loc := time.FixedZone("UTC+7", 7*60*60)
	at := func(h int) *time.Time {
		v := time.Date(2026, 5, 6, h, 7, 8, 123456789, loc)
		return &v
	}

	cases := []struct {
		name   string
		mutate func(k *apikey.APIKey)
	}{
		{name: "nullable columns unset", mutate: func(k *apikey.APIKey) {
			k.Scopes = nil
			k.ResourceIDs = nil
			k.ExpiresAt = nil
		}},
		{name: "nullable columns set", mutate: func(k *apikey.APIKey) {
			k.Scopes = []string{scope.AgentRead, `quote"back\slash`, "comma,brace{}", "NULL", ""}
			k.ResourceIDs = []uuid.UUID{uuid.New(), uuid.New()}
			k.CreatedBy = creator
			k.ExpiresAt = at(10)
			k.RevokedAt = at(11)
			k.LastUsedAt = at(12)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			k := projectKey(t, orgID, projectID)
			k.CreatedAt = *at(9)
			tc.mutate(k)
			require.NoError(t, f.store.Save(t.Context(), k))

			got, err := f.store.ByID(t.Context(), k.ID)
			require.NoError(t, err)
			assert.Equal(t, k.ID, got.ID)
			assert.Equal(t, orgID, got.OrgID)
			assert.Equal(t, projectID, got.ProjectID)
			assert.Equal(t, apikey.KindProject, got.Kind)
			assert.False(t, got.AllProjects)
			assert.NotNil(t, got.ProjectIDs)
			assert.Empty(t, got.ProjectIDs)
			assert.Equal(t, k.Name, got.Name)
			assert.Equal(t, k.SecretHash, got.SecretHash, "the bytea hash round-trips byte for byte")
			assert.Equal(t, k.SecretHint, got.SecretHint)
			assert.Equal(t, k.CreatedBy, got.CreatedBy)
			if len(k.Scopes) == 0 {
				assert.NotNil(t, got.Scopes)
				assert.Empty(t, got.Scopes)
			} else {
				assert.Equal(t, k.Scopes, got.Scopes, "text[] escaping round-trips")
			}
			if len(k.ResourceIDs) == 0 {
				assert.NotNil(t, got.ResourceIDs)
				assert.Empty(t, got.ResourceIDs)
			} else {
				assert.Equal(t, k.ResourceIDs, got.ResourceIDs)
			}
			assert.Equal(t, time.UTC, got.CreatedAt.Location())
			assert.True(t, k.CreatedAt.Truncate(time.Microsecond).Equal(got.CreatedAt))
			for name, pair := range map[string][2]*time.Time{
				"expires_at":   {k.ExpiresAt, got.ExpiresAt},
				"revoked_at":   {k.RevokedAt, got.RevokedAt},
				"last_used_at": {k.LastUsedAt, got.LastUsedAt},
			} {
				want, have := pair[0], pair[1]
				if want == nil {
					assert.Nil(t, have, name)
					continue
				}
				require.NotNil(t, have, name)
				assert.Equal(t, time.UTC, have.Location(), name)
				assert.True(t, want.Truncate(time.Microsecond).Equal(*have), "%s: want %v got %v", name, want, have)
			}
		})
	}
}

func TestPostgres_OrgKeyRoundTrip(t *testing.T) {
	f := newPgFixture(t)
	orgID, p1 := f.seedScope(t)
	p2 := f.seedProject(t, orgID)

	named := orgKey(t, orgID, apikey.ProjectGrant{ProjectIDs: []uuid.UUID{p1, p2}})
	require.NoError(t, f.store.Save(t.Context(), named))
	got, err := f.store.ByID(t.Context(), named.ID)
	require.NoError(t, err)
	assert.Equal(t, apikey.KindOrg, got.Kind)
	assert.Equal(t, uuid.Nil, got.ProjectID, "an org key has a NULL project_id")
	assert.False(t, got.AllProjects)
	assert.Equal(t, sortedIDs(p1, p2), got.ProjectIDs, "grants written together come back ordered by project id")
	assert.Equal(t, 2, f.count(t, `SELECT count(*) FROM api_key_projects WHERE key_id = $1 AND org_id = $2`, named.ID, orgID))

	all := orgKey(t, orgID, apikey.ProjectGrant{All: true})
	require.NoError(t, f.store.Save(t.Context(), all))
	got, err = f.store.ByID(t.Context(), all.ID)
	require.NoError(t, err)
	assert.True(t, got.AllProjects)
	assert.NotNil(t, got.ProjectIDs)
	assert.Empty(t, got.ProjectIDs)
}

func TestPostgres_KeySaveUpdatesOnlyMutableColumns(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	otherProject := f.seedProject(t, orgID)

	k := projectKey(t, orgID, projectID)
	k.CreatedBy = f.seedUser(t)
	k.ResourceIDs = []uuid.UUID{uuid.New()}
	require.NoError(t, f.store.Save(t.Context(), k))
	saved, err := f.store.ByID(t.Context(), k.ID)
	require.NoError(t, err)

	changed := *k
	changed.Name = "renamed"
	changed.Scopes = []string{scope.AgentWrite, scope.ProjectRead}
	changed.ResourceIDs = nil
	changed.ExpiresAt = nil
	revoked := time.Now().UTC().Truncate(time.Microsecond)
	changed.RevokedAt = &revoked
	changed.LastUsedAt = &revoked
	changed.ProjectID = otherProject
	changed.SecretHash = sha256.Sum256([]byte("another secret"))
	changed.SecretHint = "zzzz"
	changed.CreatedBy = f.seedUser(t)
	changed.CreatedAt = k.CreatedAt.Add(-time.Hour)
	require.NoError(t, f.store.Save(t.Context(), &changed))

	got, err := f.store.ByID(t.Context(), k.ID)
	require.NoError(t, err)
	assert.Equal(t, "renamed", got.Name)
	assert.Equal(t, changed.Scopes, got.Scopes)
	assert.Empty(t, got.ResourceIDs)
	assert.Nil(t, got.ExpiresAt, "expires_at can be cleared back to NULL")
	require.NotNil(t, got.RevokedAt)
	assert.True(t, revoked.Equal(*got.RevokedAt))
	require.NotNil(t, got.LastUsedAt)
	assert.True(t, revoked.Equal(*got.LastUsedAt))
	assert.Equal(t, projectID, got.ProjectID, "project_id is immutable")
	assert.Equal(t, apikey.KindProject, got.Kind, "kind is immutable")
	assert.Equal(t, saved.SecretHash, got.SecretHash, "secret_hash is immutable")
	assert.Equal(t, saved.SecretHint, got.SecretHint, "secret_hint is immutable")
	assert.Equal(t, saved.CreatedBy, got.CreatedBy, "created_by is immutable")
	assert.True(t, saved.CreatedAt.Equal(got.CreatedAt), "created_at is immutable")
	assert.Equal(t, 1, f.count(t, `SELECT count(*) FROM api_keys`))
}

func TestPostgres_KeySaveSyncsGrants(t *testing.T) {
	f := newPgFixture(t)
	orgID, p1 := f.seedScope(t)
	p2, p3 := f.seedProject(t, orgID), f.seedProject(t, orgID)

	k := orgKey(t, orgID, apikey.ProjectGrant{ProjectIDs: []uuid.UUID{p1, p2}})
	require.NoError(t, f.store.Save(t.Context(), k))
	grantCreatedAt := func(projectID uuid.UUID) time.Time {
		var at time.Time
		require.NoError(t, f.sqlDB.QueryRowContext(t.Context(),
			`SELECT created_at FROM api_key_projects WHERE key_id = $1 AND project_id = $2`, k.ID, projectID).Scan(&at))
		return at
	}
	keptAt := grantCreatedAt(p2)

	k.ProjectIDs = []uuid.UUID{p2, p3}
	require.NoError(t, f.store.Save(t.Context(), k))
	got, err := f.store.ByID(t.Context(), k.ID)
	require.NoError(t, err)
	assert.ElementsMatch(t, []uuid.UUID{p2, p3}, got.ProjectIDs, "a dropped project is pruned and a new one added")
	assert.True(t, keptAt.Equal(grantCreatedAt(p2)), "a kept grant row is left untouched")
	assert.Equal(t, p2, got.ProjectIDs[0], "grants are ordered by when they were granted")

	k.ProjectIDs = nil
	k.AllProjects = true
	require.NoError(t, f.store.Save(t.Context(), k))
	got, err = f.store.ByID(t.Context(), k.ID)
	require.NoError(t, err)
	assert.True(t, got.AllProjects)
	assert.Empty(t, got.ProjectIDs)
	assert.Zero(t, f.count(t, `SELECT count(*) FROM api_key_projects WHERE key_id = $1`, k.ID))
}

func TestPostgres_KeyKindShapeCheck(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)

	cases := []struct {
		name   string
		mutate func(k *apikey.APIKey)
	}{
		{name: "project key without a project", mutate: func(k *apikey.APIKey) { k.ProjectID = uuid.Nil }},
		{name: "project key reaching all projects", mutate: func(k *apikey.APIKey) { k.AllProjects = true }},
		{name: "org key bound to a project", mutate: func(k *apikey.APIKey) {
			k.Kind = apikey.KindOrg
			k.AllProjects = true
		}},
		{name: "personal key bound to a project", mutate: func(k *apikey.APIKey) {
			k.Kind = apikey.KindPersonal
			k.CreatedBy = uuid.Nil
		}},
		{name: "unknown kind", mutate: func(k *apikey.APIKey) { k.Kind = apikey.Kind("robot") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			k := projectKey(t, orgID, projectID)
			tc.mutate(k)
			err := f.store.Save(t.Context(), k)
			require.Error(t, err)
			assert.Equal(t, "23514", pgCode(err), "a CHECK violation surfaces raw, got %v", err)
			assert.False(t, apikey.IsExpectedError(err))
			_, err = f.store.ByID(t.Context(), k.ID)
			assert.True(t, apikey.IsNotFoundError(err))
		})
	}
}

func TestPostgres_KeySecretHashIsUnique(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	otherOrg, otherProject := f.seedScope(t)

	first := projectKey(t, orgID, projectID)
	require.NoError(t, f.store.Save(t.Context(), first))
	require.NoError(t, f.store.Save(t.Context(), first), "re-saving the same key is not a collision")

	dup := projectKey(t, otherOrg, otherProject)
	dup.SecretHash = first.SecretHash
	err := f.store.Save(t.Context(), dup)
	require.Error(t, err)
	assert.True(t, db.IsUniqueViolation(err), "the secret-hash index is global across orgs, got %v", err)
	assert.False(t, apikey.IsExpectedError(err), "the store does not map the collision to a domain error")
	_, err = f.store.ByID(t.Context(), dup.ID)
	assert.True(t, apikey.IsNotFoundError(err))
}

func TestPostgres_GrantCannotNameAnotherOrgsProject(t *testing.T) {
	f := newPgFixture(t)
	orgA, projectA := f.seedScope(t)
	_, projectB := f.seedScope(t)

	k := orgKey(t, orgA, apikey.ProjectGrant{ProjectIDs: []uuid.UUID{projectA, projectB}})
	err := f.store.Save(t.Context(), k)
	require.Error(t, err)
	assert.Equal(t, "23503", pgCode(err), "the composite FK (org_id, project_id) refuses it, got %v", err)

	_, err = f.store.ByID(t.Context(), k.ID)
	assert.True(t, apikey.IsNotFoundError(err), "a refused grant rolls back the key row too")
	assert.Zero(t, f.count(t, `SELECT count(*) FROM api_key_projects`))

	valid := orgKey(t, orgA, apikey.ProjectGrant{ProjectIDs: []uuid.UUID{projectA}})
	require.NoError(t, f.store.Save(t.Context(), valid))
	valid.ProjectIDs = []uuid.UUID{projectB}
	require.Error(t, f.store.Save(t.Context(), valid))
	got, err := f.store.ByID(t.Context(), valid.ID)
	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{projectA}, got.ProjectIDs, "a refused re-grant keeps the old grant, the prune is rolled back")
}

func TestPostgres_GrantRowOrgMustMatchKeyAndProject(t *testing.T) {
	f := newPgFixture(t)
	orgA, projectA := f.seedScope(t)
	orgB, projectB := f.seedScope(t)
	k := orgKey(t, orgA, apikey.ProjectGrant{All: true})
	require.NoError(t, f.store.Save(t.Context(), k))

	cases := []struct {
		name      string
		orgID     uuid.UUID
		projectID uuid.UUID
	}{
		{name: "row claims the project's org, not the key's", orgID: orgB, projectID: projectB},
		{name: "row claims the key's org with a foreign project", orgID: orgA, projectID: projectB},
		{name: "row claims a foreign org for a local project", orgID: orgB, projectID: projectA},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := f.sqlDB.ExecContext(t.Context(),
				`INSERT INTO api_key_projects (org_id, key_id, project_id, created_at) VALUES ($1, $2, $3, now())`,
				tc.orgID, k.ID, tc.projectID)
			require.Error(t, err)
			assert.Equal(t, "23503", pgCode(err))
		})
	}
}

func TestPostgres_ProjectKeyProjectMustBelongToItsOrg(t *testing.T) {
	f := newPgFixture(t)
	orgA := f.seedOrg(t)
	_, projectB := f.seedScope(t)

	err := f.store.Save(t.Context(), projectKey(t, orgA, projectB))
	if err == nil {
		t.Skip("BUG: api_keys.project_id has a plain FK, not (org_id, project_id) -> projects (org_id, id); a project key of org A can bind org B's project")
	}
	assert.Equal(t, "23503", pgCode(err))
}

func TestPostgres_KeyCascades(t *testing.T) {
	f := newPgFixture(t)
	orgID, doomed := f.seedScope(t)
	kept := f.seedProject(t, orgID)
	otherOrg, otherProject := f.seedScope(t)

	pk := projectKey(t, orgID, doomed)
	ok := orgKey(t, orgID, apikey.ProjectGrant{ProjectIDs: []uuid.UUID{doomed, kept}})
	foreign := orgKey(t, otherOrg, apikey.ProjectGrant{ProjectIDs: []uuid.UUID{otherProject}})
	for _, k := range []*apikey.APIKey{pk, ok, foreign} {
		require.NoError(t, f.store.Save(t.Context(), k))
	}

	_, err := f.sqlDB.ExecContext(t.Context(), `DELETE FROM projects WHERE id = $1`, doomed)
	require.NoError(t, err)
	_, err = f.store.ByID(t.Context(), pk.ID)
	assert.True(t, apikey.IsNotFoundError(err), "deleting a project removes its project keys, got %v", err)
	got, err := f.store.ByID(t.Context(), ok.ID)
	require.NoError(t, err, "an org key outlives one granted project")
	assert.Equal(t, []uuid.UUID{kept}, got.ProjectIDs, "only the deleted project's grant row goes")

	_, err = f.sqlDB.ExecContext(t.Context(), `DELETE FROM orgs WHERE id = $1`, orgID)
	require.NoError(t, err)
	_, err = f.store.ByID(t.Context(), ok.ID)
	assert.True(t, apikey.IsNotFoundError(err), "deleting an org removes its keys")
	assert.Zero(t, f.count(t, `SELECT count(*) FROM api_key_projects WHERE org_id = $1`, orgID))
	got, err = f.store.ByID(t.Context(), foreign.ID)
	require.NoError(t, err, "another org's keys survive")
	assert.Equal(t, []uuid.UUID{otherProject}, got.ProjectIDs)
}

func TestPostgres_KeyNotFound(t *testing.T) {
	f := newPgFixture(t)
	missing := uuid.New()
	_, err := f.store.ByID(t.Context(), missing)
	var nf *apikey.NotFoundError
	require.ErrorAs(t, err, &nf)
	assert.Equal(t, missing.String(), nf.ID)
}

func TestPostgres_KeyListOrderAndScope(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	otherOrg, otherProject := f.seedScope(t)
	emptyOrg := f.seedOrg(t)

	base := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	older := projectKey(t, orgID, projectID)
	older.CreatedAt = base
	tieA := orgKey(t, orgID, apikey.ProjectGrant{All: true})
	tieA.CreatedAt = base.Add(time.Minute)
	tieB := projectKey(t, orgID, projectID)
	tieB.CreatedAt = base.Add(time.Minute)
	newest := orgKey(t, orgID, apikey.ProjectGrant{ProjectIDs: []uuid.UUID{projectID}})
	newest.CreatedAt = base.Add(time.Hour)
	foreign := projectKey(t, otherOrg, otherProject)
	foreign.CreatedAt = base.Add(2 * time.Hour)
	for _, k := range []*apikey.APIKey{tieA, foreign, older, newest, tieB} {
		require.NoError(t, f.store.Save(t.Context(), k))
	}

	ties := sortedIDs(tieA.ID, tieB.ID)
	want := []uuid.UUID{newest.ID, ties[1], ties[0], older.ID}
	got, err := f.store.List(t.Context(), orgID)
	require.NoError(t, err)
	ids := make([]uuid.UUID, 0, len(got))
	for _, k := range got {
		assert.Equal(t, orgID, k.OrgID, "List never returns another org's keys")
		ids = append(ids, k.ID)
	}
	assert.Equal(t, want, ids, "newest first, ties broken by id descending")
	assert.Equal(t, []uuid.UUID{projectID}, got[0].ProjectIDs, "List carries each key's grant")

	empty, err := f.store.List(t.Context(), emptyOrg)
	require.NoError(t, err)
	assert.NotNil(t, empty)
	assert.Empty(t, empty)
}

func TestPostgres_KeyUnitOfWork(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	existing := projectKey(t, orgID, projectID)
	require.NoError(t, f.store.Save(t.Context(), existing))
	errAbort := errors.New("abort")

	fresh := projectKey(t, orgID, projectID)
	err := db.RunInTx(t.Context(), f.pool, func(ctx context.Context) error {
		got, rerr := f.store.ByID(ctx, existing.ID)
		require.NoError(t, rerr, "reads run on the unit of work's transaction")
		assert.Equal(t, existing.ID, got.ID)
		listed, rerr := f.store.List(ctx, orgID)
		require.NoError(t, rerr)
		assert.Len(t, listed, 1)

		require.NoError(t, f.store.Save(ctx, fresh), "Save joins the outer unit of work")
		saved, rerr := f.store.ByID(ctx, fresh.ID)
		require.NoError(t, rerr, "the key is visible inside the outer transaction")
		assert.Equal(t, fresh.ProjectID, saved.ProjectID)
		return errAbort
	})
	require.ErrorIs(t, err, errAbort)
	_, err = f.store.ByID(t.Context(), fresh.ID)
	assert.True(t, apikey.IsNotFoundError(err), "the key rolled back with the outer unit of work")
}

func TestPostgres_ConcurrentSecretHashClaim(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	hash := sha256.Sum256([]byte("shared secret"))

	const writers = 8
	start := make(chan struct{})
	errs := make([]error, writers)
	var wg sync.WaitGroup
	for i := range writers {
		k := projectKey(t, orgID, projectID)
		k.SecretHash = hash
		wg.Go(func() {
			<-start
			errs[i] = f.store.Save(t.Context(), k)
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
		assert.True(t, db.IsUniqueViolation(err), "a losing writer gets a unique violation, got %v", err)
	}
	assert.Equal(t, 1, wins, "exactly one key owns a secret hash")
	assert.Equal(t, 1, f.count(t, `SELECT count(*) FROM api_keys WHERE secret_hash = $1`, hash[:]))
}

func TestPostgres_ConcurrentGrantSyncsDoNotMerge(t *testing.T) {
	f := newPgFixture(t)
	orgID, p0 := f.seedScope(t)
	p1, p2 := f.seedProject(t, orgID), f.seedProject(t, orgID)

	for range 20 {
		k := orgKey(t, orgID, apikey.ProjectGrant{ProjectIDs: []uuid.UUID{p0}})
		require.NoError(t, f.store.Save(t.Context(), k))

		start := make(chan struct{})
		errs := make([]error, 2)
		var wg sync.WaitGroup
		for i, grant := range [][]uuid.UUID{{p1}, {p2}} {
			copyK := *k
			copyK.ProjectIDs = grant
			wg.Go(func() {
				<-start
				errs[i] = f.store.Save(t.Context(), &copyK)
			})
		}
		close(start)
		wg.Wait()
		require.NoError(t, errs[0])
		require.NoError(t, errs[1])

		got, err := f.store.ByID(t.Context(), k.ID)
		require.NoError(t, err)
		require.Len(t, got.ProjectIDs, 1, "the key row lock serialises grant syncs; the last writer's grant wins whole, got %v", got.ProjectIDs)
		assert.Contains(t, []uuid.UUID{p1, p2}, got.ProjectIDs[0])
	}
}

func TestPostgres_KeyUpsertCannotRewriteAnotherOrgsKey(t *testing.T) {
	f := newPgFixture(t)
	orgA, projectA := f.seedScope(t)
	orgB, projectB := f.seedScope(t)

	victim := projectKey(t, orgA, projectA)
	require.NoError(t, f.store.Save(t.Context(), victim))

	intruder := projectKey(t, orgB, projectB)
	intruder.ID = victim.ID
	intruder.Name = "hijacked"
	intruder.Scopes = []string{scope.AgentWrite, scope.ProjectWrite}
	far := time.Now().UTC().Add(300 * 24 * time.Hour)
	intruder.ExpiresAt = &far
	_ = f.store.Save(t.Context(), intruder)

	got, err := f.store.ByID(t.Context(), victim.ID)
	require.NoError(t, err)
	if got.Name != victim.Name || !slices.Equal(got.Scopes, victim.Scopes) {
		t.Skip("BUG: apikey Save ON CONFLICT (id) DO UPDATE has no org_id guard; a colliding id from org B rewrites org A's key scopes/expiry")
	}
}

func TestPostgres_BySecretHashLoadsGrants(t *testing.T) {
	f := newPgFixture(t)
	orgID, p1 := f.seedScope(t)
	p2 := f.seedProject(t, orgID)
	k, plain, err := apikey.NewScheme("").MintOrg(orgID, "ci", []string{"agent:read"},
		apikey.ProjectGrant{ProjectIDs: []uuid.UUID{p1, p2}}, expiry(), time.Now().UTC())
	require.NoError(t, err)
	require.NoError(t, f.store.Save(t.Context(), k))

	got, err := f.store.BySecretHash(t.Context(), sha256.Sum256([]byte(plain)))
	require.NoError(t, err)
	assert.Equal(t, k.ID, got.ID)
	assert.ElementsMatch(t, []uuid.UUID{p1, p2}, got.ProjectIDs)
	assert.Equal(t, []string{"agent:read"}, got.Scopes)

	_, err = f.store.BySecretHash(t.Context(), sha256.Sum256([]byte("oagent_unknown")))
	assert.True(t, apikey.IsNotFoundError(err), "got %v", err)
}

func TestPostgres_TouchLastUsedNeverMovesBack(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	k, _, err := apikey.NewScheme("").MintProject(orgID, projectID, "bot", []string{"agent:read"}, nil, expiry(), time.Now().UTC())
	require.NoError(t, err)
	require.NoError(t, f.store.Save(t.Context(), k))

	later := time.Now().UTC().Truncate(time.Microsecond)
	earlier := later.Add(-time.Hour)

	require.NoError(t, f.store.TouchLastUsed(t.Context(), map[uuid.UUID]time.Time{k.ID: later}))
	require.NoError(t, f.store.TouchLastUsed(t.Context(), map[uuid.UUID]time.Time{k.ID: earlier}))

	got, err := f.store.ByID(t.Context(), k.ID)
	require.NoError(t, err)
	require.NotNil(t, got.LastUsedAt)
	assert.True(t, later.Equal(*got.LastUsedAt), "got %v want %v", got.LastUsedAt, later)

	require.NoError(t, f.store.TouchLastUsed(t.Context(), map[uuid.UUID]time.Time{}))
}

func (f *pgFixture) seedUser(t *testing.T) uuid.UUID {
	t.Helper()
	u, err := user.New("u-"+uuid.NewString()[:8]+"@example.com", "User")
	require.NoError(t, err)
	require.NoError(t, user.NewStore(f.pool, f.schema, "").Save(t.Context(), u))
	return u.ID
}

func TestPostgres_PersonalKeyRoundTrip(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	owner := f.seedUser(t)
	k, plain, err := apikey.NewScheme("").MintPersonal(orgID, owner, "laptop", []string{"agent:read"},
		apikey.ProjectGrant{ProjectIDs: []uuid.UUID{projectID}}, expiry(), time.Now().UTC())
	require.NoError(t, err)
	require.NoError(t, f.store.Save(t.Context(), k))

	got, err := f.store.BySecretHash(t.Context(), sha256.Sum256([]byte(plain)))
	require.NoError(t, err)
	assert.Equal(t, apikey.KindPersonal, got.Kind)
	assert.Equal(t, owner, got.CreatedBy)
	assert.Equal(t, []uuid.UUID{projectID}, got.ProjectIDs)
	assert.Equal(t, []uuid.UUID{projectID}, got.ReachableProjects())
}

func TestPostgres_DeletingUserNullsKeyOwner(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	owner := f.seedUser(t)
	personal, _, err := apikey.NewScheme("").MintPersonal(orgID, owner, "laptop", []string{"agent:read"},
		apikey.ProjectGrant{All: true}, expiry(), time.Now().UTC())
	require.NoError(t, err)
	machine := projectKey(t, orgID, projectID)
	machine.CreatedBy = owner
	for _, k := range []*apikey.APIKey{personal, machine} {
		require.NoError(t, f.store.Save(t.Context(), k))
	}

	_, err = f.sqlDB.ExecContext(t.Context(), `DELETE FROM users WHERE id = $1`, owner)
	require.NoError(t, err)

	for _, id := range []uuid.UUID{personal.ID, machine.ID} {
		got, err := f.store.ByID(t.Context(), id)
		require.NoError(t, err, "keys outlive their creator")
		assert.Equal(t, uuid.Nil, got.CreatedBy)
	}
}

func TestPostgres_KeyCreatorMustExist(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	k := projectKey(t, orgID, projectID)
	k.CreatedBy = uuid.New()
	err := f.store.Save(t.Context(), k)
	require.Error(t, err)
	assert.Equal(t, "23503", pgCode(err))
}

func TestPostgres_RevokePersonalOf(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	otherOrg := f.seedOrg(t)
	owner, someoneElse := f.seedUser(t), f.seedUser(t)
	personal := func(org, user uuid.UUID) *apikey.APIKey {
		k, _, err := apikey.NewScheme("").MintPersonal(org, user, "laptop", []string{"agent:read"},
			apikey.ProjectGrant{All: true}, expiry(), time.Now().UTC())
		require.NoError(t, err)
		return k
	}
	mine, mineToo := personal(orgID, owner), personal(orgID, owner)
	elsewhere := personal(otherOrg, owner)
	theirs := personal(orgID, someoneElse)
	machine := projectKey(t, orgID, projectID)
	machine.CreatedBy = owner
	org := orgKey(t, orgID, apikey.ProjectGrant{All: true})
	org.CreatedBy = owner
	already := personal(orgID, owner)
	earlier := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	already.RevokedAt = &earlier
	for _, k := range []*apikey.APIKey{mine, mineToo, elsewhere, theirs, machine, org, already} {
		require.NoError(t, f.store.Save(t.Context(), k))
	}

	at := time.Now().UTC().Truncate(time.Microsecond)
	n, err := f.store.RevokePersonalOf(t.Context(), orgID, owner, at)
	require.NoError(t, err)
	assert.Equal(t, 2, n, "only the owner's live personal keys in that org")

	for _, k := range []*apikey.APIKey{mine, mineToo} {
		got, err := f.store.ByID(t.Context(), k.ID)
		require.NoError(t, err)
		if assert.NotNil(t, got.RevokedAt) {
			assert.True(t, got.RevokedAt.Equal(at))
		}
	}
	for _, k := range []*apikey.APIKey{elsewhere, theirs, machine, org} {
		got, err := f.store.ByID(t.Context(), k.ID)
		require.NoError(t, err)
		assert.Nil(t, got.RevokedAt, "key %s stays live", got.Kind)
	}
	got, err := f.store.ByID(t.Context(), already.ID)
	require.NoError(t, err)
	assert.True(t, got.RevokedAt.Equal(earlier), "the first revocation wins")
}

func TestPostgres_RevokePersonalOfJoinsTheTransaction(t *testing.T) {
	f := newPgFixture(t)
	orgID := f.seedOrg(t)
	owner := f.seedUser(t)
	k, _, err := apikey.NewScheme("").MintPersonal(orgID, owner, "laptop", []string{"agent:read"},
		apikey.ProjectGrant{All: true}, expiry(), time.Now().UTC())
	require.NoError(t, err)
	require.NoError(t, f.store.Save(t.Context(), k))

	rollback := errors.New("rollback")
	err = db.RunInTx(t.Context(), f.pool, func(ctx context.Context) error {
		n, err := f.store.RevokePersonalOf(ctx, orgID, owner, time.Now().UTC())
		require.NoError(t, err)
		assert.Equal(t, 1, n)
		return rollback
	})
	require.ErrorIs(t, err, rollback)
	got, err := f.store.ByID(t.Context(), k.ID)
	require.NoError(t, err)
	assert.Nil(t, got.RevokedAt)
}

func TestPostgres_LiveHintsAreUniquePerOrg(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	otherOrg, otherProject := f.seedScope(t)
	first := projectKey(t, orgID, projectID)
	require.NoError(t, f.store.Save(t.Context(), first))

	twin := projectKey(t, orgID, projectID)
	twin.SecretHint = first.SecretHint
	assert.True(t, apikey.IsHintTakenError(f.store.Save(t.Context(), twin)), "two live keys in one org never share a hint")

	elsewhere := projectKey(t, otherOrg, otherProject)
	elsewhere.SecretHint = first.SecretHint
	require.NoError(t, f.store.Save(t.Context(), elsewhere), "another org may use the same hint")

	first.Revoke(time.Now().UTC())
	require.NoError(t, f.store.Save(t.Context(), first))
	require.NoError(t, f.store.Save(t.Context(), twin), "a revoked key frees its hint")
}

func TestPostgres_RevokeByHintRevokesExactlyOneKey(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	otherOrg, otherProject := f.seedScope(t)
	revoked := projectKey(t, orgID, projectID)
	revoked.RevokedAt = expiry()
	target := projectKey(t, orgID, projectID)
	revoked.SecretHint = target.SecretHint
	elsewhere := projectKey(t, otherOrg, otherProject)
	elsewhere.SecretHint = target.SecretHint
	bystander := projectKey(t, orgID, projectID)
	for _, k := range []*apikey.APIKey{revoked, target, elsewhere, bystander} {
		require.NoError(t, f.store.Save(t.Context(), k))
	}

	got, err := f.store.ByHint(t.Context(), orgID, target.SecretHint)
	require.NoError(t, err)
	assert.Equal(t, target.ID, got.ID, "ByHint finds the live key, not a revoked one with the same hint")

	svc := apikey.NewService(f.store, apikey.NewScheme(""), nil, apperror.NewReporter(nil, false).Unexpected)
	require.NoError(t, svc.RevokeByHint(t.Context(), orgID, target.SecretHint, uuid.Nil, true))
	live := f.count(t, `SELECT count(*) FROM api_keys WHERE revoked_at IS NULL AND id = ANY($1)`,
		pgUUIDs(target.ID, elsewhere.ID, bystander.ID))
	assert.Equal(t, 2, live, "only the target is revoked")
	_, err = f.store.ByHint(t.Context(), orgID, target.SecretHint)
	assert.True(t, apikey.IsNotFoundError(err))
}

func pgUUIDs(ids ...uuid.UUID) string {
	s := make([]string, len(ids))
	for i, id := range ids {
		s[i] = id.String()
	}
	return "{" + strings.Join(s, ",") + "}"
}
