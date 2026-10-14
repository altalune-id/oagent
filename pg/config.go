package pg

import (
	"errors"
	"time"
)

// Config is one Postgres connection: the DSN, the role set on every new connection, the pool and the connect retry.
type Config struct {
	DSN             string
	Role            string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
	ConnectTimeout  time.Duration
	ConnectBackoff  time.Duration
}

// HealthConfig tunes the health monitor.
type HealthConfig struct {
	Interval time.Duration
	Timeout  time.Duration
}

// Validate rejects configs that cannot open a connection.
func (c Config) Validate() error {
	if c.DSN == "" {
		return errors.New("pg: dsn is required")
	}
	return nil
}
