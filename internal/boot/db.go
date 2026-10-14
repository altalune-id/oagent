package boot

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/kernel"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/migrator"
	"altalune.id/oagent/pg"
	"altalune.id/oagent/schema"
)

func buildDBHealth(k kernel.Kernel, cfg config.Config, log *slog.Logger) *pg.HealthMonitor {
	h := cfg.Postgres.Health
	return pg.NewHealthMonitor(k.Pool.W, k.Pool.R, pg.HealthConfig{Interval: h.Interval, Timeout: h.Timeout}, log, k.Meter)
}

type (
	openPostgresFunc func(context.Context, pg.Config, *slog.Logger) (*sql.DB, error)
	openPoolFunc     func(context.Context, config.PostgresConfig, *slog.Logger) (db.Pool, error)
	openMigratorFunc func(context.Context, string, migrator.Config) (*sql.DB, error)
	migrateUpFunc    func(context.Context, *sql.DB, migrator.Config) error
	withLockFunc     func(context.Context, *sql.DB, string, func(context.Context) error) error
)

//nolint:gochecknoglobals // test seams.
var (
	openPostgres openPostgresFunc = pg.Open
	openPool     openPoolFunc     = openPostgresPool
	openMigrator openMigratorFunc = migrator.Open
	migrateUp    migrateUpFunc    = schema.MigrateUp
	withLock     withLockFunc     = pg.RunWithAdvisoryLock
)

func openDB(ctx context.Context, cfg config.Config, log *slog.Logger) (db.Pool, error) {
	switch cfg.DB.Driver {
	case "", config.DBDriverPostgres:
		return openPool(ctx, cfg.Postgres, log)
	default:
		return db.Pool{}, fmt.Errorf("boot: db.driver %q is not supported", cfg.DB.Driver)
	}
}

func openPostgresPool(ctx context.Context, c config.PostgresConfig, log *slog.Logger) (db.Pool, error) {
	writer, err := openPostgres(ctx, writerConfig(c), log)
	if err != nil {
		return db.Pool{}, err
	}
	if c.Reader.DSN == "" {
		return db.Pool{W: writer, R: writer}, nil
	}
	reader, err := openPostgres(ctx, readerConfig(c), log)
	if err != nil {
		_ = writer.Close()
		return db.Pool{}, err
	}
	return db.Pool{W: writer, R: reader}, nil
}

func writerConfig(c config.PostgresConfig) pg.Config {
	return pg.Config{
		DSN:             c.DSN,
		Role:            c.Role,
		MaxOpenConns:    c.MaxOpenConns,
		MaxIdleConns:    c.MaxIdleConns,
		ConnMaxLifetime: c.ConnMaxLifetime,
		ConnMaxIdleTime: c.ConnMaxIdleTime,
		ConnectTimeout:  c.ConnectTimeout,
		ConnectBackoff:  c.ConnectBackoff,
	}
}

func readerConfig(c config.PostgresConfig) pg.Config {
	return pg.Config{
		DSN:             c.Reader.DSN,
		Role:            c.Reader.Role,
		MaxOpenConns:    c.Reader.MaxOpenConns,
		MaxIdleConns:    c.Reader.MaxIdleConns,
		ConnMaxLifetime: c.Reader.ConnMaxLifetime,
		ConnMaxIdleTime: c.Reader.ConnMaxIdleTime,
		ConnectTimeout:  c.ConnectTimeout,
		ConnectBackoff:  c.ConnectBackoff,
	}
}

// NOTE: runs under a session advisory lock so concurrent instances migrate once.
func migrate(ctx context.Context, cfg config.PostgresConfig) error {
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

func migratorDSN(c config.PostgresConfig) string {
	if c.Migrator.DSN != "" {
		return c.Migrator.DSN
	}
	return c.DSN
}
