//go:build integration

package pgtest_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/testutil/pgtest"
)

func TestOpenDB_HandlesDoNotShareASchema(t *testing.T) {
	a, b := pgtest.New(t), pgtest.New(t)
	require.True(t, strings.HasPrefix(a.Schema, "pgt_"), "schema %q must be namespaced", a.Schema)
	require.NotEqual(t, a.Schema, b.Schema)
	assert.Equal(t, a.DSN, b.DSN, "handles in one test binary share one server")

	dbA, dbB := a.OpenDB(t), b.OpenDB(t)
	_, err := dbA.ExecContext(t.Context(), `CREATE TABLE isolation_probe (id INT)`)
	require.NoError(t, err)

	var schema string
	require.NoError(t, dbA.QueryRowContext(t.Context(),
		`SELECT schemaname FROM pg_tables WHERE tablename = 'isolation_probe'`).Scan(&schema))
	assert.Equal(t, a.Schema, schema, "OpenDB must place objects in the handle's schema, not public")

	var visible bool
	require.NoError(t, dbB.QueryRowContext(t.Context(),
		`SELECT EXISTS (SELECT 1 FROM pg_tables
		   WHERE tablename = 'isolation_probe' AND schemaname = ANY (current_schemas(false)))`).Scan(&visible))
	assert.False(t, visible, "one handle must not see another handle's tables")
}

func TestOpenDB_DropsItsSchemaWhenTheTestEnds(t *testing.T) {
	probe := pgtest.New(t).OpenDB(t)
	var inner string
	t.Run("inner", func(t *testing.T) {
		h := pgtest.New(t)
		inner = h.Schema
		_, err := h.OpenDB(t).ExecContext(t.Context(), `CREATE TABLE leak_probe (id INT)`)
		require.NoError(t, err)
	})

	var exists bool
	require.NoError(t, probe.QueryRowContext(context.Background(),
		`SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = $1)`, inner).Scan(&exists))
	assert.False(t, exists, "the subtest's schema must be dropped with its contents")
}

func TestMigrated_AppliesEveryMigration(t *testing.T) {
	sqlDB, schemaName := pgtest.Migrated(t)
	var n int
	require.NoError(t, sqlDB.QueryRowContext(t.Context(),
		`SELECT count(*) FROM information_schema.tables WHERE table_schema = $1 AND table_name = 'orgs'`, schemaName).Scan(&n))
	assert.Equal(t, 1, n)
}
