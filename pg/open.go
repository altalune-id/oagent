package pg

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5"
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
	if err := checkLoginRole(ctx, db, cfg.Role); err != nil {
		_ = db.Close()
		return nil, err
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
	return stdlib.OpenDB(*connCfg), nil
}

func checkLoginRole(ctx context.Context, db *sql.DB, want string) error {
	if want == "" {
		return nil
	}
	var got string
	if err := db.QueryRowContext(ctx, "SELECT session_user").Scan(&got); err != nil {
		return fmt.Errorf("pg: login role: %w", err)
	}
	if got != want {
		return &LoginRoleError{Want: want, Got: got}
	}
	return nil
}
