//go:build integration

package pg_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/testutil/pgtest"
	"altalune.id/oagent/pg"
)

func currentUsers(t *testing.T, sqlDB *sql.DB) (current, session string) {
	t.Helper()
	require.NoError(t, sqlDB.QueryRowContext(t.Context(), `SELECT current_user, session_user`).Scan(&current, &session))
	return current, session
}

func createRole(t *testing.T, dsn string) string {
	t.Helper()
	admin := openReplica(t, dsn)
	role := "pgt_role_" + strings.ReplaceAll(strings.ToLower(t.Name()), "/", "_")
	role = role[:min(len(role), 63)]
	quoted := pgx.Identifier{role}.Sanitize()
	_, _ = admin.ExecContext(t.Context(), "DROP ROLE IF EXISTS "+quoted)
	if _, err := admin.ExecContext(t.Context(), "CREATE ROLE "+quoted+" NOLOGIN"); err != nil {
		t.Skipf("cannot create a role on this server: %v", err)
	}
	t.Cleanup(func() {
		if _, err := admin.ExecContext(context.Background(), "DROP ROLE IF EXISTS "+quoted); err != nil {
			t.Errorf("drop role %s: %v", role, err)
		}
	})
	return role
}

func TestOpen_AppliesPoolTuning(t *testing.T) {
	sqlDB, err := pg.Open(t.Context(), pg.Config{DSN: pgtest.New(t).DSN, MaxOpenConns: 3, MaxIdleConns: 2}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	assert.Equal(t, 3, sqlDB.Stats().MaxOpenConnections)
	require.NoError(t, sqlDB.PingContext(t.Context()))
}

func TestOpen_SetsTheRoleOnEveryConnection(t *testing.T) {
	dsn := pgtest.New(t).DSN
	role := createRole(t, dsn)
	login, _ := currentUsers(t, openReplica(t, dsn))

	const conns = 3
	sqlDB, err := pg.Open(t.Context(), pg.Config{DSN: dsn, Role: role, MaxOpenConns: conns}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	held := make([]*sql.Conn, 0, conns)
	t.Cleanup(func() {
		for _, c := range held {
			_ = c.Close()
		}
	})
	for range conns {
		c, cErr := sqlDB.Conn(t.Context())
		require.NoError(t, cErr)
		held = append(held, c)
		var current, session string
		require.NoError(t, c.QueryRowContext(t.Context(), `SELECT current_user, session_user`).Scan(&current, &session))
		assert.Equal(t, role, current)
		assert.Equal(t, login, session)
	}
	assert.Equal(t, conns, sqlDB.Stats().OpenConnections)
}

func TestOpen_UnknownRoleFailsFastWithoutRetrying(t *testing.T) {
	dsn := pgtest.New(t).DSN
	start := time.Now()
	_, err := pg.Open(t.Context(), pg.Config{
		DSN: dsn, Role: "pgt_role_does_not_exist", ConnectTimeout: 20 * time.Second,
	}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SET ROLE")
	assert.Less(t, time.Since(start), 10*time.Second, "a SET ROLE rejected by the server is permanent and must not be retried")
}

func TestOpen_InvalidRoleIdentifierIsRejectedBeforeConnecting(t *testing.T) {
	_, err := pg.Open(t.Context(), pg.Config{DSN: pgtest.New(t).DSN, Role: "bad\nrole"}, nil)
	require.Error(t, err)
	assert.True(t, pg.IsInvalidIdentifierError(err), "got %v", err)
}

func TestOpen_UnreachableServerRetriesUntilTheBudget(t *testing.T) {
	budget := 1500 * time.Millisecond
	start := time.Now()
	_, err := pg.Open(t.Context(), pg.Config{
		DSN:            "postgres://u:p@127.0.0.1:1/x?sslmode=disable&connect_timeout=1",
		ConnectTimeout: budget, ConnectBackoff: 100 * time.Millisecond,
	}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pg: ping")
	elapsed := time.Since(start)
	assert.GreaterOrEqual(t, elapsed, budget-100*time.Millisecond, "a transient failure must be retried within the budget")
	assert.Less(t, elapsed, budget+3*time.Second)
}
