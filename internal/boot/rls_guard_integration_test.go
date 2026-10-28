//go:build integration

package boot

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/testutil/pgtest"
)

func rlsGuardConfig(schema string) config.Config {
	return config.Config{HTTP: config.HTTPConfig{Addr: ":5150"}, Env: config.EnvDevelopment, Mode: config.ModeSelfhosted, Postgres: config.PostgresConfig{Schema: schema}}
}

func TestGuardRLS_TheServiceRolePasses(t *testing.T) {
	svc, _, schemaName := pgtest.MigratedRLS(t)
	require.NoError(t, guardRLS(t.Context(), svc, rlsGuardConfig(schemaName), testLogger()))
}

func TestGuardRLS_RefusesASuperuserOnEitherPool(t *testing.T) {
	svc, admin, schemaName := pgtest.MigratedRLS(t)

	err := guardRLS(t.Context(), db.Pool{W: admin, R: admin}, rlsGuardConfig(schemaName), testLogger())
	require.True(t, db.IsRLSError(err), "got %v", err)
	assert.Contains(t, err.Error(), "superuser")
	assert.Contains(t, err.Error(), "writer")

	err = guardRLS(t.Context(), db.Pool{W: svc.W, R: admin}, rlsGuardConfig(schemaName), testLogger())
	require.True(t, db.IsRLSError(err), "got %v", err)
	assert.Contains(t, err.Error(), "reader")
}

func TestGuardRLS_AllowBypassWarnsButNeverRelaxesTheTables(t *testing.T) {
	_, admin, schemaName := pgtest.MigratedRLS(t)
	cfg := rlsGuardConfig(schemaName)
	cfg.DB.RLS.AllowBypass = true
	var logs bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logs, nil))

	require.NoError(t, guardRLS(t.Context(), db.Pool{W: admin, R: admin}, cfg, log))
	assert.Contains(t, logs.String(), "level=WARN")
	assert.Contains(t, logs.String(), "db.rls.allowBypass")

	_, err := admin.ExecContext(t.Context(), "ALTER TABLE "+pgx.Identifier{schemaName, "projects"}.Sanitize()+" NO FORCE ROW LEVEL SECURITY")
	require.NoError(t, err)
	err = guardRLS(t.Context(), db.Pool{W: admin, R: admin}, cfg, log)
	require.True(t, db.IsRLSError(err), "got %v", err)
	assert.Contains(t, err.Error(), "table projects: FORCE ROW LEVEL SECURITY is missing")
}

func TestGuardRLS_RefusesAPolicyThatIgnoresTheTenant(t *testing.T) {
	svc, admin, schemaName := pgtest.MigratedRLS(t)
	orgs := pgx.Identifier{schemaName, "orgs"}.Sanitize()
	_, err := admin.ExecContext(t.Context(), "INSERT INTO "+orgs+" (id, public_id, slug, name, created_at, updated_at) "+
		"VALUES (gen_random_uuid(), 'leakprobe', 'leak-probe', 'Leak probe', now(), now())")
	require.NoError(t, err)
	require.NoError(t, guardRLS(t.Context(), svc, rlsGuardConfig(schemaName), testLogger()), "a sound policy over real rows passes")

	_, err = admin.ExecContext(t.Context(), "ALTER POLICY orgs_read ON "+orgs+" USING ("+
		pgx.Identifier{schemaName, "current_org_id"}.Sanitize()+"() = id OR true)")
	require.NoError(t, err)
	err = guardRLS(t.Context(), svc, rlsGuardConfig(schemaName), testLogger())
	require.True(t, db.IsRLSError(err), "got %v", err)
	assert.Contains(t, err.Error(), "table orgs: a scope no tenant owns still sees rows")
}
