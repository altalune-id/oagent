//go:build integration

package pgtest

import (
	"context"
	"database/sql"
	"fmt"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

const vectorAdvisoryKey = 7_301_955_118

//nolint:gochecknoglobals // the extension is server-wide: create it once per server per test binary.
var vectorExt struct {
	mu   sync.Mutex
	done map[string]error
}

func vectorOnce(ctx context.Context, dsn string) error {
	vectorExt.mu.Lock()
	defer vectorExt.mu.Unlock()
	if err, ok := vectorExt.done[dsn]; ok {
		return err
	}
	err := ensureVector(ctx, dsn)
	if vectorExt.done == nil {
		vectorExt.done = map[string]error{}
	}
	vectorExt.done[dsn] = err
	return err
}

func ensureVector(ctx context.Context, dsn string) error {
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return fmt.Errorf("parse DSN: %w", err)
	}
	admin := stdlib.OpenDB(*cfg)
	defer func() { _ = admin.Close() }()
	return createVector(ctx, admin)
}

func createVector(ctx context.Context, admin *sql.DB) error {
	tx, err := admin.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", int64(vectorAdvisoryKey)); err != nil {
		return fmt.Errorf("advisory lock: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public"); err != nil {
		return fmt.Errorf("create extension vector: %w", err)
	}
	return tx.Commit()
}
