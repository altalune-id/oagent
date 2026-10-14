package config

import (
	"strings"
	"time"
)

// DBDriver selects the database behind the stores, the outbox, the mail queue and the sessions.
type DBDriver string

// DBDriverPostgres keeps every table in Postgres.
const DBDriverPostgres DBDriver = "postgres"

// DBConfig picks the database driver and whether the schema is migrated at start.
type DBConfig struct {
	Driver      DBDriver `yaml:"driver"      mapstructure:"driver"      validate:"oneof=postgres"`
	AutoMigrate bool     `yaml:"autoMigrate" mapstructure:"autoMigrate"`
}

// PostgresConfig is the Postgres connection; it is opened only when a feature selects driver postgres.
type PostgresConfig struct {
	DSN             string                 `yaml:"dsn"             mapstructure:"dsn"             awareness:"required,secret"`
	Role            string                 `yaml:"role"            mapstructure:"role"`
	Schema          string                 `yaml:"schema"          mapstructure:"schema"          awareness:"bootstrap"`
	TablePrefix     string                 `yaml:"tablePrefix"     mapstructure:"tablePrefix"     awareness:"bootstrap"`
	MaxOpenConns    int                    `yaml:"maxOpenConns"    mapstructure:"maxOpenConns"    validate:"gte=0"`
	MaxIdleConns    int                    `yaml:"maxIdleConns"    mapstructure:"maxIdleConns"    validate:"gte=0"`
	ConnMaxLifetime time.Duration          `yaml:"connMaxLifetime" mapstructure:"connMaxLifetime"`
	ConnMaxIdleTime time.Duration          `yaml:"connMaxIdleTime" mapstructure:"connMaxIdleTime"`
	ConnectTimeout  time.Duration          `yaml:"connectTimeout"  mapstructure:"connectTimeout"`
	ConnectBackoff  time.Duration          `yaml:"connectBackoff"  mapstructure:"connectBackoff"`
	Reader          PostgresReaderConfig   `yaml:"reader"          mapstructure:"reader"`
	Migrator        PostgresMigratorConfig `yaml:"migrator"        mapstructure:"migrator"`
	Health          PostgresHealthConfig   `yaml:"health"          mapstructure:"health"`
}

// PostgresReaderConfig points reads at a replica; an empty DSN falls back to the writer.
type PostgresReaderConfig struct {
	DSN             string        `yaml:"dsn"             mapstructure:"dsn"             awareness:"secret"`
	Role            string        `yaml:"role"            mapstructure:"role"`
	MaxOpenConns    int           `yaml:"maxOpenConns"    mapstructure:"maxOpenConns"    validate:"gte=0"`
	MaxIdleConns    int           `yaml:"maxIdleConns"    mapstructure:"maxIdleConns"    validate:"gte=0"`
	ConnMaxLifetime time.Duration `yaml:"connMaxLifetime" mapstructure:"connMaxLifetime"`
	ConnMaxIdleTime time.Duration `yaml:"connMaxIdleTime" mapstructure:"connMaxIdleTime"`
}

// PostgresMigratorConfig is a separate credential used only to run migrations; an empty DSN uses postgres.dsn.
type PostgresMigratorConfig struct {
	DSN  string `yaml:"dsn"  mapstructure:"dsn"  awareness:"secret"`
	Role string `yaml:"role" mapstructure:"role"`
}

// PostgresHealthConfig tunes the Postgres health monitor.
type PostgresHealthConfig struct {
	Interval time.Duration `yaml:"interval" mapstructure:"interval" validate:"gte=0"`
	Timeout  time.Duration `yaml:"timeout"  mapstructure:"timeout"  validate:"gte=0"`
}

// PostgresFeatures lists the settings that select Postgres; empty means oagent never connects to it.
func (c *Config) PostgresFeatures() []string {
	var out []string
	if c.DB.Driver == DBDriverPostgres {
		out = append(out, "db.driver")
	}
	return out
}

func (c *Config) validatePostgres() error {
	features := c.PostgresFeatures()
	if len(features) == 0 {
		return nil
	}
	if c.Postgres.DSN == "" {
		return &MissingSettingError{Keys: "postgres.dsn", Reason: strings.Join(features, ", ") + " is postgres"}
	}
	return nil
}
