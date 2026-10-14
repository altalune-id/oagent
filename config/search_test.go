package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeConfig(t *testing.T, dir, name, body string) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	p := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
	return p
}

func TestFindConfigFile_XDG(t *testing.T) {
	xdg := t.TempDir()
	want := writeConfig(t, filepath.Join(xdg, "oagent"), "config.yaml", "postgres:\n  dsn: postgres://xdg/db\n")
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Chdir(t.TempDir())

	assert.Equal(t, want, findConfigFile())
}

func TestFindConfigFile_Cwd(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	wd := t.TempDir()
	want := writeConfig(t, wd, "config.yaml", "postgres:\n  dsn: postgres://cwd/db\n")
	t.Chdir(wd)

	assert.Equal(t, want, findConfigFile())
}

func TestConfigCandidates_Priority(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Chdir(t.TempDir())

	cands := configCandidates()

	require.NotEmpty(t, cands)
	assert.Equal(t, filepath.Join(xdg, "oagent", "config.yaml"), cands[0])
}

func TestLoad_SearchPath(t *testing.T) {
	setSecretEnv(t)
	xdg := t.TempDir()
	writeConfig(t, filepath.Join(xdg, "oagent"), "config.yaml", "postgres:\n  dsn: postgres://search/db\n")
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Chdir(t.TempDir())

	c, err := Load("")

	require.NoError(t, err)
	assert.Equal(t, "postgres://search/db", c.Postgres.DSN)
}

func TestLoader_BindFlag_OverridesEnv(t *testing.T) {
	setSecretEnv(t)
	t.Setenv("OAGENT_MODE", "cloud")
	t.Setenv("OAGENT_POSTGRES_DSN", "postgres://env/db")

	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	fs.String("mode", "", "")
	fs.String("postgres-dsn", "", "")
	require.NoError(t, fs.Set("mode", "selfhosted"))
	require.NoError(t, fs.Set("postgres-dsn", "postgres://flag/db"))

	l := NewLoader()
	require.NoError(t, l.BindFlag("mode", fs.Lookup("mode")))
	require.NoError(t, l.BindFlag("postgres.dsn", fs.Lookup("postgres-dsn")))

	c, err := l.Load(filepath.Join(t.TempDir(), "none.yaml"))

	require.NoError(t, err)
	assert.Equal(t, ModeSelfhosted, c.Mode)
	assert.Equal(t, "postgres://flag/db", c.Postgres.DSN)
}

func TestLoader_BindFlag_Nil(t *testing.T) {
	assert.Error(t, NewLoader().BindFlag("postgres.dsn", nil))
}
