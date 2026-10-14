//go:build integration

package schema_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/testutil/pgtest"
	"altalune.id/oagent/migrator"
	"altalune.id/oagent/schema"
)

type twoOrgs struct {
	orgA, orgB, prjA, prjB, endpointA uuid.UUID
}

func seedTwoOrgs(t *testing.T, db *sql.DB) twoOrgs {
	t.Helper()
	ctx := t.Context()
	s := twoOrgs{orgA: uuid.New(), orgB: uuid.New(), prjA: uuid.New(), prjB: uuid.New(), endpointA: uuid.New()}
	for _, o := range []uuid.UUID{s.orgA, s.orgB} {
		_, err := db.ExecContext(ctx, `INSERT INTO orgs (id, public_id, slug, name, created_at, updated_at) VALUES ($1, $2, $2, 'Org', now(), now())`,
			o, "o"+o.String()[:8])
		require.NoError(t, err)
	}
	for prj, org := range map[uuid.UUID]uuid.UUID{s.prjA: s.orgA, s.prjB: s.orgB} {
		_, err := db.ExecContext(ctx, `INSERT INTO projects (id, public_id, org_id, slug, name, created_at, updated_at) VALUES ($1, $2, $3, 'p', 'P', now(), now())`,
			prj, "p"+prj.String()[:8], org)
		require.NoError(t, err)
	}
	_, err := db.ExecContext(ctx, `INSERT INTO webhook_endpoints (id, public_id, org_id, project_id, url, event_types, secret_primary, active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 'https://e.com', 'agent.created', '\x01', true, now(), now())`, s.endpointA, "w"+s.endpointA.String()[:8], s.orgA, s.prjA)
	require.NoError(t, err)
	return s
}

func inserts(s twoOrgs) map[string]func(ctx context.Context, db *sql.DB, org, prj uuid.UUID) error {
	exec := func(q string, extra ...any) func(ctx context.Context, db *sql.DB, org, prj uuid.UUID) error {
		return func(ctx context.Context, db *sql.DB, org, prj uuid.UUID) error {
			id := uuid.New()
			_, err := db.ExecContext(ctx, q, append([]any{id, org, prj, id.String()}, extra...)...)
			return err
		}
	}
	return map[string]func(ctx context.Context, db *sql.DB, org, prj uuid.UUID) error{
		"agents": exec(`INSERT INTO agents (id, public_id, org_id, project_id, slug, name, created_at, updated_at)
			VALUES ($1, $4, $2, $3, $4, 'A', now(), now())`),
		"webhook_endpoints": exec(`INSERT INTO webhook_endpoints (id, public_id, org_id, project_id, url, event_types, secret_primary, active, created_at, updated_at)
			VALUES ($1, $4, $2, $3, 'https://e.com', 'agent.created', '\x01', true, now(), now())`),
		"webhook_deliveries": exec(`INSERT INTO webhook_deliveries (id, org_id, project_id, endpoint_id, delivery_id, event_id, event_type, attempt, duration_ms, error, created_at)
			VALUES ($1, $2, $3, $5, $1, $1, 'agent.created', 1, 1, $4, now())`, s.endpointA),
		"outbox_entries": exec(`INSERT INTO outbox_entries (id, event_id, org_id, project_id, target, payload, next_attempt_at, last_error, created_at, public_id, event_public_id)
			VALUES ($1, $1, $2, $3, 'webhook', '\x01', now(), $4, now(), gen_random_uuid()::text, 'e')`),
		"api_keys": exec(`INSERT INTO api_keys (id, org_id, project_id, kind, name, secret_hash, secret_hint, created_at)
			VALUES ($1, $2, $3, 'project', 'k', decode(md5($4), 'hex'), substr(md5($4), 1, 4), now())`),
	}
}

func pgCode(err error) string {
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		return pgErr.Code
	}
	return ""
}

func TestProjectFK_RejectsAProjectFromAnotherOrg(t *testing.T) {
	db, _ := pgtest.Migrated(t)
	s := seedTwoOrgs(t, db)
	for table, insert := range inserts(s) {
		t.Run(table, func(t *testing.T) {
			require.NoError(t, insert(t.Context(), db, s.orgA, s.prjA), "own project is fine")
			err := insert(t.Context(), db, s.orgA, s.prjB)
			require.Error(t, err)
			assert.Equal(t, "23503", pgCode(err), "foreign key violation, got %v", err)
		})
	}
}

func TestProjectFK_NullProjectStaysAllowed(t *testing.T) {
	db, _ := pgtest.Migrated(t)
	s := seedTwoOrgs(t, db)
	_, err := db.ExecContext(t.Context(), `INSERT INTO api_keys (id, org_id, kind, all_projects, name, secret_hash, secret_hint, created_at)
		VALUES ($1, $2, 'org', true, 'k', '\x02', 'abcd', now())`, uuid.New(), s.orgA)
	require.NoError(t, err)
}

func TestProjectFK_DeletingAProjectStillCascades(t *testing.T) {
	db, _ := pgtest.Migrated(t)
	s := seedTwoOrgs(t, db)
	for _, insert := range inserts(s) {
		require.NoError(t, insert(t.Context(), db, s.orgA, s.prjA))
	}
	_, err := db.ExecContext(t.Context(), `DELETE FROM projects WHERE id = $1`, s.prjA)
	require.NoError(t, err)
	for table := range inserts(s) {
		var n int
		require.NoError(t, db.QueryRowContext(t.Context(), `SELECT count(*) FROM `+table+` WHERE project_id = $1`, s.prjA).Scan(&n))
		assert.Zero(t, n, table)
	}
}

func TestProjectFK_MigrationRefusesExistingCrossOrgRows(t *testing.T) {
	h := pgtest.New(t)
	db := h.OpenDB(t)
	cfg := migrator.Config{Schema: h.Schema}
	ctx := t.Context()
	require.NoError(t, schema.MigrateUp(ctx, db, cfg))
	require.NoError(t, schema.MigrateDownTo(ctx, db, cfg, 11))

	s := seedTwoOrgs(t, db)
	require.NoError(t, inserts(s)["agents"](ctx, db, s.orgA, s.prjB), "version 11 still allows it")

	err := schema.MigrateUp(ctx, db, cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agents")
	assert.Contains(t, err.Error(), "project from another org")

	var n int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM agents`).Scan(&n))
	assert.Equal(t, 1, n, "the bad row is left for a person to fix")
}

func TestProjectFK_OneCompositeKeyPerTable(t *testing.T) {
	db, schemaName := pgtest.Migrated(t)
	for table := range inserts(twoOrgs{}) {
		var n, cols int
		require.NoError(t, db.QueryRowContext(t.Context(), `
			SELECT count(*), coalesce(max(array_length(c.conkey, 1)), 0)
			FROM pg_constraint c
			WHERE c.contype = 'f' AND c.conrelid = ($1 || '.' || $2)::regclass AND c.confrelid = ($1 || '.projects')::regclass`,
			schemaName, table).Scan(&n, &cols))
		assert.Equal(t, 1, n, "%s: one foreign key to projects", table)
		assert.Equal(t, 2, cols, "%s: on (org_id, project_id)", table)
	}
}
