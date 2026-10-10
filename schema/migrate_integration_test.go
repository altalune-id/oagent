//go:build integration

package schema_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/testutil/pgtest"
	"altalune.id/oagent/migrator"
	"altalune.id/oagent/schema"
)

func TestMigrations_UpDownUp(t *testing.T) {
	h := pgtest.New(t)
	sqlDB := h.OpenDB(t)
	cfg := migrator.Config{Schema: h.Schema, TablePrefix: "t_"}
	ctx := t.Context()

	require.NoError(t, schema.MigrateUp(ctx, sqlDB, cfg))
	want, err := schema.New(cfg).TargetVersion()
	require.NoError(t, err)
	ok, err := schema.New(cfg).IsUpToDate(ctx, sqlDB)
	require.NoError(t, err)
	assert.True(t, ok, "up to VERSION %d", want)

	require.NoError(t, schema.MigrateDownTo(ctx, sqlDB, cfg, 0), "every Down must run")
	var left []string
	rows, err := sqlDB.QueryContext(ctx,
		`SELECT tablename FROM pg_tables WHERE schemaname = $1 AND tablename <> 't_goose_db_version' ORDER BY 1`, h.Schema)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		left = append(left, name)
	}
	require.NoError(t, rows.Err())
	assert.Empty(t, left, "Down must drop everything Up created")

	require.NoError(t, schema.MigrateUp(ctx, sqlDB, cfg), "Up must work again after a full Down")
}

func TestMigrations_TwoPrefixesShareASchema(t *testing.T) {
	h := pgtest.New(t)
	sqlDB := h.OpenDB(t)
	for _, prefix := range []string{"a_", "b_"} {
		require.NoError(t, schema.MigrateUp(t.Context(), sqlDB, migrator.Config{Schema: h.Schema, TablePrefix: prefix}),
			"prefix %q: every table, index and constraint name must carry the prefix", prefix)
	}
}
