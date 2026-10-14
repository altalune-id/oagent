package boot

import (
	"context"
	"database/sql"
	"log/slog"

	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/kernel"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/migrator"
	"altalune.id/oagent/schema"
)

func buildDBHealth(k kernel.Kernel, cfg config.Config, log *slog.Logger) *db.HealthMonitor {
	return db.NewHealthMonitor(k.Pool, dbConfig(cfg.DB).Health, log, k.Meter)
}

type (
	openPoolFunc     func(context.Context, db.DBConfig, *slog.Logger) (db.Pool, error)
	openMigratorFunc func(context.Context, string, migrator.Config) (*sql.DB, error)
	migrateUpFunc    func(context.Context, *sql.DB, migrator.Config) error
	withLockFunc     func(context.Context, *sql.DB, string, func(context.Context) error) error
)

//nolint:gochecknoglobals // test seams.
var (
	openPool     openPoolFunc     = db.OpenPool
	openMigrator openMigratorFunc = migrator.Open
	migrateUp    migrateUpFunc    = schema.MigrateUp
	withLock     withLockFunc     = db.RunWithAdvisoryLock
)

// NOTE: runs under a session advisory lock so concurrent instances migrate once.
func migrate(ctx context.Context, cfg config.DBConfig) error {
	migCfg := migrator.Config{
		Schema:      cfg.Schema,
		TablePrefix: cfg.TablePrefix,
		Role:        cfg.Migrator.Role,
	}
	migDB, err := openMigrator(ctx, migratorDSN(cfg), migCfg)
	if err != nil {
		return err
	}
	defer func() { _ = migDB.Close() }()

	return withLock(ctx, migDB, MigrationLockName(cfg.TablePrefix), func(ctx context.Context) error {
		return migrateUp(ctx, migDB, migCfg)
	})
}

// MigrationLockName is the advisory-lock name that serialises migrate runs sharing one table prefix.
func MigrationLockName(prefix string) string { return "migrate:" + prefix }

func migratorDSN(c config.DBConfig) string {
	if c.Migrator.DSN != "" {
		return c.Migrator.DSN
	}
	return c.DSN
}

func dbConfig(c config.DBConfig) db.DBConfig {
	return db.DBConfig{
		DSN:         c.DSN,
		Schema:      c.Schema,
		TablePrefix: c.TablePrefix,
		Role:        c.Role,
		Reader: db.ReaderConfig{
			DSN:             c.Reader.DSN,
			Role:            c.Reader.Role,
			MaxOpenConns:    c.Reader.MaxOpenConns,
			MaxIdleConns:    c.Reader.MaxIdleConns,
			ConnMaxLifetime: c.Reader.ConnMaxLifetime,
			ConnMaxIdleTime: c.Reader.ConnMaxIdleTime,
		},
		Migrator:        db.MigratorConfig{DSN: c.Migrator.DSN, Role: c.Migrator.Role},
		Health:          db.HealthConfig{Interval: c.Health.Interval, Timeout: c.Health.Timeout},
		MaxOpenConns:    c.MaxOpenConns,
		MaxIdleConns:    c.MaxIdleConns,
		ConnMaxLifetime: c.ConnMaxLifetime,
		ConnMaxIdleTime: c.ConnMaxIdleTime,
		ConnectTimeout:  c.ConnectTimeout,
		ConnectBackoff:  c.ConnectBackoff,
	}
}
