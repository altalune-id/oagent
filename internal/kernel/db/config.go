package db

import (
	"fmt"
	"time"
)

// DBConfig is the input to Open.
type DBConfig struct {
	DSN             string
	Schema          string
	TablePrefix     string
	Role            string
	Reader          ReaderConfig
	Migrator        MigratorConfig
	Health          HealthConfig
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
	ConnectTimeout  time.Duration
	ConnectBackoff  time.Duration
}

// ReaderConfig points reads at a replica; an empty DSN falls back to the writer.
type ReaderConfig struct {
	DSN             string
	Role            string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

// MigratorConfig is a separate credential used only to run migrations.
type MigratorConfig struct {
	DSN  string
	Role string
}

// HealthConfig tunes the db-health worker.
type HealthConfig struct {
	Interval time.Duration
	Timeout  time.Duration
}

// Validate rejects configs that cannot open a connection.
func (c DBConfig) Validate() error {
	if c.DSN == "" {
		return fmt.Errorf("db: dsn is required")
	}
	return nil
}
