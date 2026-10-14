package pg

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"hash/fnv"
	"time"
)

const unlockBudget = 5 * time.Second

// LockKey derives a stable advisory-lock key from name.
func LockKey(name string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(name))
	return int64(h.Sum64()) //nolint:gosec // wrapping to int64 is intentional; pg_advisory_lock takes a signed bigint.
}

// RunWithAdvisoryLock runs fn while holding a session-scoped Postgres advisory lock, so concurrent replicas serialise.
func RunWithAdvisoryLock(ctx context.Context, sqldb *sql.DB, name string, fn func(context.Context) error) error {
	conn, err := sqldb.Conn(ctx)
	if err != nil {
		return fmt.Errorf("pg: lock conn: %w", err)
	}
	key := LockKey(name)
	if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", key); err != nil {
		_ = conn.Close()
		return fmt.Errorf("pg: advisory lock %q: %w", name, err)
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), unlockBudget)
		defer cancel()
		if _, err := conn.ExecContext(unlockCtx, "SELECT pg_advisory_unlock($1)", key); err != nil {
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		}
		_ = conn.Close()
	}()
	return fn(ctx)
}
