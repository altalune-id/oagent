package migrator

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"altalune.id/oagent/pg"
)

// Open opens a two-session migrator *sql.DB (lock + goose/upgrades) and applies SET ROLE when cfg.Role is set.
func Open(ctx context.Context, dsn string, cfg Config) (*sql.DB, error) {
	connCfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("migrator: parse dsn: %w", err)
	}
	var db *sql.DB
	if cfg.Role == "" {
		db = stdlib.OpenDB(*connCfg)
	} else {
		if err := pg.ValidateIdentifier(cfg.Role); err != nil {
			return nil, err
		}
		stmt := "SET ROLE " + pg.QuoteIdentifier(cfg.Role)
		afterConnect := func(ctx context.Context, conn *pgx.Conn) error {
			if _, err := conn.Exec(ctx, stmt); err != nil {
				return fmt.Errorf("migrator: SET ROLE %q: %w", cfg.Role, err)
			}
			return nil
		}
		db = stdlib.OpenDB(*connCfg, stdlib.OptionAfterConnect(afterConnect))
	}
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(2)
	return db, nil
}
