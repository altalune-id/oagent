// Package schema embeds the oagent migrations and wires them to migrator.
package schema

import (
	"context"
	"database/sql"
	"embed"
	"io/fs"

	"altalune.id/oagent/migrator"
)

//go:embed migrations
var migrationsFS embed.FS

//nolint:gochecknoglobals // immutable embedded FS, computed once.
var migrations, _ = fs.Sub(migrationsFS, "migrations")

// New builds a migrator over the embedded Postgres migrations.
func New(cfg migrator.Config) *migrator.Migrator {
	return migrator.New(migrations, cfg)
}

// MigrationsFS returns the raw embedded migrations directory, unrendered.
func MigrationsFS() fs.FS { return migrations }

// MigrateUp applies pending migrations.
func MigrateUp(ctx context.Context, db *sql.DB, cfg migrator.Config) error {
	return New(cfg).Up(ctx, db)
}

// MigrateStatus reports migration status.
func MigrateStatus(ctx context.Context, db *sql.DB, cfg migrator.Config) ([]migrator.MigrationStatus, error) {
	return New(cfg).Status(ctx, db)
}

// MigrateDownTo rolls back to version.
func MigrateDownTo(ctx context.Context, db *sql.DB, cfg migrator.Config, version int64) error {
	return New(cfg).DownTo(ctx, db, version)
}
