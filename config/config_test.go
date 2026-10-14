package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func missingPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "none.yaml")
}

func TestLoad_Defaults(t *testing.T) {
	setSecretEnv(t)
	t.Setenv("OAGENT_POSTGRES_DSN", "postgres://localhost/db")

	c, err := Load(missingPath(t))
	require.NoError(t, err)
	assert.Equal(t, "postgres://localhost/db", c.Postgres.DSN)
	assert.Equal(t, "public", c.Postgres.Schema)
	assert.True(t, c.DB.AutoMigrate) // default
	assert.Equal(t, 5*time.Second, c.Postgres.ConnectTimeout)
	assert.Equal(t, ModeSelfhosted, c.Mode) // default
	assert.Equal(t, EnvProduction, c.Env)   // default
	assert.True(t, c.Env.IsProduction())
	assert.Empty(t, c.HTTP.BasePath)               // default
	assert.True(t, c.API.OpenAPI.Enabled)          // default
	assert.True(t, c.API.OpenAPI.RequireBasicAuth) // default
	assert.True(t, c.HTTP.CookieSecure)            // default
}

func TestLoad_BasePathOverride(t *testing.T) {
	setSecretEnv(t)
	t.Setenv("OAGENT_POSTGRES_DSN", "postgres://localhost/db")
	t.Setenv("OAGENT_HTTP_BASE_PATH", "/app")

	c, err := Load(missingPath(t))
	require.NoError(t, err)
	assert.Equal(t, "/app", c.HTTP.BasePath)
}

func TestLoad_ModeAndEnvOverride(t *testing.T) {
	setSecretEnv(t)
	t.Setenv("OAGENT_POSTGRES_DSN", "postgres://localhost/db")
	t.Setenv("OAGENT_MODE", "cloud")
	t.Setenv("OAGENT_ENV", "production")

	_, err := Load(missingPath(t))
	assert.True(t, IsCloudRequiresOIDCError(err), "cloud must refuse to boot until OIDC (P3); got %v", err)

	t.Setenv("OAGENT_MODE", "selfhosted")
	c, err := Load(missingPath(t))
	require.NoError(t, err)
	assert.Equal(t, ModeSelfhosted, c.Mode)
	assert.True(t, c.Env.IsProduction())
}

func TestLoad_InvalidMode(t *testing.T) {
	t.Setenv("OAGENT_POSTGRES_DSN", "postgres://localhost/db")
	t.Setenv("OAGENT_MODE", "bogus")

	_, err := Load(missingPath(t))
	assert.Error(t, err)
}

func TestLoad_InvalidEnv(t *testing.T) {
	t.Setenv("OAGENT_POSTGRES_DSN", "postgres://localhost/db")
	t.Setenv("OAGENT_ENV", "bogus")

	_, err := Load(missingPath(t))
	assert.Error(t, err)
}

func TestLoad_AutoMigrateOverride(t *testing.T) {
	setSecretEnv(t)
	t.Setenv("OAGENT_POSTGRES_DSN", "postgres://localhost/db")
	t.Setenv("OAGENT_DB_AUTO_MIGRATE", "false")

	c, err := Load(missingPath(t))
	require.NoError(t, err)
	assert.False(t, c.DB.AutoMigrate)
}

func TestLoad_DefaultPath(t *testing.T) {
	setSecretEnv(t)
	t.Setenv("OAGENT_POSTGRES_DSN", "postgres://localhost/db")

	c, err := Load("")
	require.NoError(t, err)
	assert.Equal(t, "postgres://localhost/db", c.Postgres.DSN)
}

func TestLoad_InvalidTelemetry(t *testing.T) {
	t.Setenv("OAGENT_POSTGRES_DSN", "postgres://localhost/db")
	t.Setenv("OAGENT_TELEMETRY_OTLP_PROTOCOL", "bogus")

	_, err := Load(missingPath(t))
	assert.Error(t, err)
}

func TestLoad_MissingDSN(t *testing.T) {
	t.Setenv("OAGENT_POSTGRES_DSN", "")

	_, err := Load(missingPath(t))
	assert.Error(t, err)
}

func TestLoad_EnvOverride(t *testing.T) {
	setSecretEnv(t)
	t.Setenv("OAGENT_POSTGRES_DSN", "postgres://localhost/db")
	t.Setenv("OAGENT_POSTGRES_SCHEMA", "auth")
	t.Setenv("OAGENT_POSTGRES_TABLE_PREFIX", "t_")
	t.Setenv("OAGENT_TELEMETRY_TRACING_ENABLED", "true")

	c, err := Load(missingPath(t))
	require.NoError(t, err)
	assert.Equal(t, "auth", c.Postgres.Schema)
	assert.Equal(t, "t_", c.Postgres.TablePrefix)
	assert.True(t, c.Telemetry.Tracing.Enabled)
}

func TestLoad_File(t *testing.T) {
	setSecretEnv(t)
	path := filepath.Join(t.TempDir(), "oagent.yaml")
	require.NoError(t, os.WriteFile(path, []byte("postgres:\n  dsn: postgres://file/db\n  tablePrefix: f_\n"), 0o600))
	t.Setenv("OAGENT_POSTGRES_DSN", "") // no env override; take the file value

	c, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "postgres://file/db", c.Postgres.DSN)
	assert.Equal(t, "f_", c.Postgres.TablePrefix)
}

func TestLoad_RequireFileMissing(t *testing.T) {
	_, err := Load(missingPath(t), WithRequireFile())
	assert.Error(t, err)
}
