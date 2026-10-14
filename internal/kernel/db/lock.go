package db

import (
	"context"
	"fmt"

	"altalune.id/oagent/pg"
)

// LockInTx takes a transaction-scoped advisory lock on the unit of work in ctx; Postgres releases it at commit or rollback.
func LockInTx(ctx context.Context, name string) error {
	tx, ok := CurrentTx(ctx)
	if !ok {
		return fmt.Errorf("db: advisory xact lock %q: no transaction in context", name)
	}
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", pg.LockKey(name)); err != nil {
		return fmt.Errorf("db: advisory xact lock %q: %w", name, err)
	}
	return nil
}
