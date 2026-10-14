package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_DBAndPostgresDefaults(t *testing.T) {
	c := validConfig(t)
	assert.Equal(t, DBDriverPostgres, c.DB.Driver)
	assert.True(t, c.DB.AutoMigrate)
	assert.Equal(t, "postgres://localhost/db", c.Postgres.DSN)
	assert.Equal(t, "public", c.Postgres.Schema)
	assert.Empty(t, c.Postgres.TablePrefix)
	assert.Equal(t, 5*time.Second, c.Postgres.ConnectTimeout)
	assert.Equal(t, 250*time.Millisecond, c.Postgres.ConnectBackoff)
	assert.Equal(t, 30*time.Second, c.Postgres.Health.Interval)
	assert.Equal(t, 2*time.Second, c.Postgres.Health.Timeout)
}

func TestLoad_PostgresFromEnv(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("OAGENT_POSTGRES_SCHEMA", "auth")
	t.Setenv("OAGENT_POSTGRES_TABLE_PREFIX", "t_")
	t.Setenv("OAGENT_POSTGRES_ROLE", "app")
	t.Setenv("OAGENT_POSTGRES_READER_DSN", "postgres://replica/db")
	t.Setenv("OAGENT_POSTGRES_MIGRATOR_DSN", "postgres://owner/db")
	t.Setenv("OAGENT_POSTGRES_MIGRATOR_ROLE", "owner")
	t.Setenv("OAGENT_POSTGRES_HEALTH_INTERVAL", "10s")
	t.Setenv("OAGENT_DB_AUTO_MIGRATE", "false")
	c, err := Load(missingPath(t))
	require.NoError(t, err)
	assert.Equal(t, "auth", c.Postgres.Schema)
	assert.Equal(t, "t_", c.Postgres.TablePrefix)
	assert.Equal(t, "app", c.Postgres.Role)
	assert.Equal(t, "postgres://replica/db", c.Postgres.Reader.DSN)
	assert.Equal(t, "postgres://owner/db", c.Postgres.Migrator.DSN)
	assert.Equal(t, "owner", c.Postgres.Migrator.Role)
	assert.Equal(t, 10*time.Second, c.Postgres.Health.Interval)
	assert.False(t, c.DB.AutoMigrate)
}

func TestPostgresFeatures(t *testing.T) {
	c := validConfig(t)
	assert.Equal(t, []string{"db.driver"}, c.PostgresFeatures())
	c.DB.Driver = "sqlite"
	assert.Empty(t, c.PostgresFeatures())
}

func TestValidate_Postgres(t *testing.T) {
	for name, tc := range map[string]struct {
		mutate func(c *Config)
		check  func(t *testing.T, err error)
	}{
		"defaults ok": {
			mutate: func(*Config) {},
			check:  valid,
		},
		"dsn empty names the feature": {
			mutate: func(c *Config) { c.Postgres.DSN = "" },
			check: func(t *testing.T, err error) {
				missingSetting("postgres.dsn")(t, err)
				assert.ErrorContains(t, err, "db.driver")
			},
		},
		"driver unknown": {
			mutate: func(c *Config) { c.DB.Driver = "sqlite" },
			check:  invalidSetting("db.driver"),
		},
		"driver empty": {
			mutate: func(c *Config) { c.DB.Driver = "" },
			check:  invalidSetting("db.driver"),
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := validConfig(t)
			tc.mutate(c)
			tc.check(t, c.Validate())
		})
	}
}

func TestLoad_DBFromFile(t *testing.T) {
	setRequiredEnv(t)
	path := filepath.Join(t.TempDir(), "oagent.yaml")
	require.NoError(t, os.WriteFile(path, []byte("db:\n  driver: postgres\n  autoMigrate: false\n"), 0o600))
	t.Setenv("OAGENT_DB_DRIVER", "postgres")
	c, err := Load(path)
	require.NoError(t, err)
	assert.False(t, c.DB.AutoMigrate)
}

func TestWalkEnvKeys_PostgresAwareness(t *testing.T) {
	byYAML := map[string]EnvKey{}
	for _, k := range WalkEnvKeys(EnvPrefix) {
		byYAML[k.YAML] = k
	}
	for key, want := range map[string][]string{
		"postgres.dsn":          {"required", "secret"},
		"postgres.reader.dsn":   {"secret"},
		"postgres.migrator.dsn": {"secret"},
		"postgres.schema":       {"bootstrap"},
		"postgres.tablePrefix":  {"bootstrap"},
	} {
		k, ok := byYAML[key]
		require.True(t, ok, key)
		assert.ElementsMatch(t, want, k.Awareness, key)
	}
	assert.Equal(t, "OAGENT_POSTGRES_DSN", byYAML["postgres.dsn"].Key)
	assert.Equal(t, "OAGENT_DB_DRIVER", byYAML["db.driver"].Key)
	assert.Equal(t, "postgres", byYAML["db.driver"].FormatDefault())
	for _, gone := range []string{"db.dsn", "db.schema", "db.tablePrefix", "db.migrator.dsn", "rateLimit.store"} {
		assert.NotContains(t, byYAML, gone)
	}
}
