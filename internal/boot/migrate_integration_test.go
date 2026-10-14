//go:build integration

package boot

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/testutil/pgtest"
)

const unreachableDSN = "postgres://app:app-secret@127.0.0.1:1/none?sslmode=disable&connect_timeout=1"

func migrateRole(t *testing.T, admin *sql.DB, schemaName string) string {
	t.Helper()
	role := "pgt_mig_" + strings.ToLower(schemaName)
	role = role[:min(len(role), 63)]
	quoted := pgx.Identifier{role}.Sanitize()
	for _, stmt := range []string{
		"CREATE ROLE " + quoted + " NOLOGIN",
		"GRANT " + quoted + " TO CURRENT_USER",
		"GRANT ALL ON SCHEMA " + pgx.Identifier{schemaName}.Sanitize() + " TO " + quoted,
	} {
		if _, err := admin.ExecContext(t.Context(), stmt); err != nil {
			t.Skipf("cannot set up a migrator role on this server: %v", err)
		}
	}
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = admin.ExecContext(ctx, "DROP OWNED BY "+quoted)
		if _, err := admin.ExecContext(ctx, "DROP ROLE IF EXISTS "+quoted); err != nil {
			t.Errorf("drop role %s: %v", role, err)
		}
	})
	return role
}

func recordLock(t *testing.T) *[]string {
	t.Helper()
	restoreSeams(t)
	var names []string
	lock := withLock
	withLock = func(ctx context.Context, sqlDB *sql.DB, name string, fn func(context.Context) error) error {
		names = append(names, name)
		return lock(ctx, sqlDB, name, fn)
	}
	return &names
}

func tableOwner(t *testing.T, admin *sql.DB, schemaName, table string) string {
	t.Helper()
	var owner string
	require.NoError(t, admin.QueryRowContext(t.Context(),
		`SELECT tableowner FROM pg_tables WHERE schemaname = $1 AND tablename = $2`, schemaName, table).Scan(&owner))
	return owner
}

func TestMigrate_UsesTheMigratorCredentialAndRole(t *testing.T) {
	h := pgtest.New(t)
	admin := h.OpenDB(t)
	role := migrateRole(t, admin, h.Schema)
	locks := recordLock(t)

	err := migrate(t.Context(), config.PostgresConfig{
		DSN:         unreachableDSN,
		Schema:      h.Schema,
		TablePrefix: "t_",
		Migrator:    config.PostgresMigratorConfig{DSN: h.DSN, Role: role},
	})
	require.NoError(t, err)

	assert.Equal(t, role, tableOwner(t, admin, h.Schema, "t_orgs"), "the migrator role owns what it creates")
	assert.Equal(t, []string{"migrate:t_"}, *locks)
}

func TestMigrate_WithoutAMigratorUsesTheAppCredential(t *testing.T) {
	h := pgtest.New(t)
	admin := h.OpenDB(t)
	locks := recordLock(t)

	err := migrate(t.Context(), config.PostgresConfig{DSN: h.DSN, Schema: h.Schema, TablePrefix: "u_"})
	require.NoError(t, err)

	connCfg, err := pgx.ParseConfig(h.DSN)
	require.NoError(t, err)
	login := stdlib.OpenDB(*connCfg)
	t.Cleanup(func() { _ = login.Close() })
	var user string
	require.NoError(t, login.QueryRowContext(t.Context(), `SELECT current_user`).Scan(&user))

	assert.Equal(t, user, tableOwner(t, admin, h.Schema, "u_orgs"))
	assert.Equal(t, []string{"migrate:u_"}, *locks)
}
