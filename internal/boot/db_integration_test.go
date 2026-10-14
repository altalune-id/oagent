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

func loginDB(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	connCfg, err := pgx.ParseConfig(dsn)
	require.NoError(t, err)
	sqlDB := stdlib.OpenDB(*connCfg)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return sqlDB
}

func currentUser(t *testing.T, sqlDB *sql.DB) string {
	t.Helper()
	var user string
	require.NoError(t, sqlDB.QueryRowContext(t.Context(), `SELECT current_user`).Scan(&user))
	return user
}

func grantedRole(t *testing.T, admin *sql.DB) string {
	t.Helper()
	role := "pgt_role_" + strings.ToLower(strings.ReplaceAll(t.Name(), "/", "_"))
	role = role[:min(len(role), 63)]
	quoted := pgx.Identifier{role}.Sanitize()
	_, _ = admin.ExecContext(t.Context(), "DROP ROLE IF EXISTS "+quoted)
	for _, stmt := range []string{"CREATE ROLE " + quoted + " NOLOGIN", "GRANT " + quoted + " TO CURRENT_USER"} {
		if _, err := admin.ExecContext(t.Context(), stmt); err != nil {
			t.Skipf("cannot create a role on this server: %v", err)
		}
	}
	t.Cleanup(func() {
		if _, err := admin.ExecContext(context.Background(), "DROP ROLE IF EXISTS "+quoted); err != nil {
			t.Errorf("drop role %s: %v", role, err)
		}
	})
	return role
}

func TestOpenPool_ReaderGetsItsOwnHandleAndRole(t *testing.T) {
	dsn := pgtest.New(t).DSN
	admin := loginDB(t, dsn)
	role := grantedRole(t, admin)
	login := currentUser(t, admin)

	pool, err := openPostgresPool(t.Context(), config.PostgresConfig{
		DSN: dsn, Role: role, MaxOpenConns: 4,
		Reader: config.PostgresReaderConfig{DSN: dsn, MaxOpenConns: 2},
	}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pool.Close() })

	require.NotSame(t, pool.W, pool.R)
	assert.Equal(t, role, currentUser(t, pool.W))
	assert.Equal(t, login, currentUser(t, pool.R), "the reader must not inherit the writer's role")
	assert.Equal(t, 4, pool.W.Stats().MaxOpenConnections)
	assert.Equal(t, 2, pool.R.Stats().MaxOpenConnections)

	require.NoError(t, pool.Close())
	require.Error(t, pool.W.PingContext(t.Context()))
	require.Error(t, pool.R.PingContext(t.Context()))
}

func TestOpenPool_WithoutReaderAliasesTheWriter(t *testing.T) {
	pool, err := openPostgresPool(t.Context(), config.PostgresConfig{DSN: pgtest.New(t).DSN}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pool.Close() })
	assert.Same(t, pool.W, pool.R)
}
