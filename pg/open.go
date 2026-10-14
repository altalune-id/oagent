package pg

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"
)

//nolint:gochecknoglobals // test seam.
var openFunc = openPostgres

// Open opens the Postgres *sql.DB, pings it with bounded retry, and applies pool tuning.
func Open(ctx context.Context, cfg Config, log *slog.Logger) (*sql.DB, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	db, err := openFunc(cfg)
	if err != nil {
		return nil, err
	}
	pingErr := retry(withRetryLogger(ctx, log), cfg.ConnectTimeout, cfg.ConnectBackoff,
		func(attemptCtx context.Context) error { return db.PingContext(attemptCtx) })
	if pingErr != nil {
		_ = db.Close()
		return nil, fmt.Errorf("pg: ping: %w", pingErr)
	}

	if cfg.MaxOpenConns > 0 {
		db.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		db.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLifetime > 0 {
		db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	}
	if cfg.ConnMaxIdleTime > 0 {
		db.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)
	}

	if log != nil {
		log.DebugContext(ctx, "db opened",
			slog.Int("max_open_conns", cfg.MaxOpenConns),
			slog.Int("max_idle_conns", cfg.MaxIdleConns),
		)
	}
	return db, nil
}

func openPostgres(cfg Config) (*sql.DB, error) {
	connCfg, err := pgx.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("pg: parse dsn: %w", err)
	}
	connCfg.Tracer = otelpgx.NewTracer()
	if cfg.Role == "" {
		return stdlib.OpenDB(*connCfg), nil
	}
	if err := ValidateIdentifier(cfg.Role); err != nil {
		return nil, err
	}
	stmt := "SET ROLE " + QuoteIdentifier(cfg.Role)
	afterConnect := func(ctx context.Context, conn *pgx.Conn) error {
		if _, execErr := conn.Exec(ctx, stmt); execErr != nil {
			wrapped := fmt.Errorf("pg: SET ROLE %q: %w", cfg.Role, execErr)
			if _, ok := errors.AsType[*pgconn.PgError](execErr); ok {
				return permanent(wrapped)
			}
			return wrapped
		}
		return nil
	}
	return stdlib.OpenDB(*connCfg, stdlib.OptionAfterConnect(afterConnect)), nil
}
