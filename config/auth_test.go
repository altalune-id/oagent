package config

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/password"
)

func validConfig(t *testing.T) *Config {
	t.Helper()
	t.Setenv("OAGENT_DB_DSN", "postgres://localhost/db")
	c, err := Load(missingPath(t))
	require.NoError(t, err)
	return c
}

func TestLoad_AuthDefaults(t *testing.T) {
	c := validConfig(t)
	assert.Equal(t, AuthModeLocal, c.Auth.Mode)
	assert.Equal(t, 24*time.Hour, c.Auth.SessionTTL)
	assert.Empty(t, c.Install.Token)
}

func TestValidate_Auth(t *testing.T) {
	for name, tc := range map[string]struct {
		mutate func(c *Config)
		check  func(t *testing.T, err error)
	}{
		"local selfhosted ok": {
			mutate: func(*Config) {},
			check:  func(t *testing.T, err error) { assert.NoError(t, err) },
		},
		"basic with credentials ok": {
			mutate: func(c *Config) {
				c.Auth.Mode = AuthModeBasic
				c.Auth.Basic = BasicConfig{User: "ops", Password: "s3cret-pass"}
			},
			check: func(t *testing.T, err error) { assert.NoError(t, err) },
		},
		"basic without credentials": {
			mutate: func(c *Config) { c.Auth.Mode = AuthModeBasic },
			check:  func(t *testing.T, err error) { assert.True(t, IsMissingSettingError(err), "got %v", err) },
		},
		"basic short password": {
			mutate: func(c *Config) {
				c.Auth.Mode = AuthModeBasic
				c.Auth.Basic = BasicConfig{User: "ops", Password: "short"}
			},
			check: func(t *testing.T, err error) { assert.True(t, password.IsPolicyError(err), "got %v", err) },
		},
		"basic in cloud": {
			mutate: func(c *Config) {
				c.Mode = ModeCloud
				c.Auth.Mode = AuthModeBasic
				c.Auth.Basic = BasicConfig{User: "ops", Password: "s3cret-pass"}
			},
			check: func(t *testing.T, err error) { assert.True(t, IsAuthModeError(err), "got %v", err) },
		},
		"genesis email only": {
			mutate: func(c *Config) { c.Auth.Genesis.Email = "root@example.com" },
			check:  func(t *testing.T, err error) { assert.True(t, IsMissingSettingError(err), "got %v", err) },
		},
		"genesis short password": {
			mutate: func(c *Config) { c.Auth.Genesis = GenesisConfig{Email: "root@example.com", Password: "short"} },
			check:  func(t *testing.T, err error) { assert.True(t, password.IsPolicyError(err), "got %v", err) },
		},
		"genesis ok": {
			mutate: func(c *Config) { c.Auth.Genesis = GenesisConfig{Email: "root@example.com", Password: "long-enough"} },
			check:  func(t *testing.T, err error) { assert.NoError(t, err) },
		},
		"genesis in cloud": {
			mutate: func(c *Config) {
				c.Mode = ModeCloud
				c.Auth.Genesis = GenesisConfig{Email: "root@example.com", Password: "long-enough"}
			},
			check: func(t *testing.T, err error) { assert.True(t, IsGenesisInCloudError(err), "got %v", err) },
		},
		"cloud requires oidc": {
			mutate: func(c *Config) { c.Mode = ModeCloud },
			check: func(t *testing.T, err error) {
				assert.True(t, IsCloudRequiresOIDCError(err), "got %v", err)
				assert.True(t, strings.Contains(err.Error(), "OIDC"), err.Error())
			},
		},
		"session ttl too short": {
			mutate: func(c *Config) { c.Auth.SessionTTL = 30 * time.Minute },
			check:  func(t *testing.T, err error) { assert.Error(t, err) },
		},
		"unknown auth mode": {
			mutate: func(c *Config) { c.Auth.Mode = "magic" },
			check:  func(t *testing.T, err error) { assert.Error(t, err) },
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := validConfig(t)
			tc.mutate(c)
			tc.check(t, c.Validate())
		})
	}
}

func TestErrors_Messages(t *testing.T) {
	assert.Equal(t, `config: auth.mode "basic" is not allowed with mode "cloud"`, (&AuthModeError{AuthMode: AuthModeBasic, Mode: ModeCloud}).Error())
	assert.Equal(t, "config: auth.basic.user required: auth.mode is basic", (&MissingSettingError{Keys: "auth.basic.user", Reason: "auth.mode is basic"}).Error())
	assert.Contains(t, (&GenesisInCloudError{}).Error(), "selfhosted only")
}
