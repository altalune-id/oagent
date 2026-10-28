package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/password"
)

const (
	testStateSecret   = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	testEncryptionKey = "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"
	testMailFrom      = "oagent <noreply@example.com>"
)

func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("OAGENT_POSTGRES_DSN", "postgres://localhost/db")
	setSecretEnv(t)
}

func setSecretEnv(t *testing.T) {
	t.Helper()
	t.Setenv("OAGENT_HTTP_STATE_SECRET", testStateSecret)
	t.Setenv("OAGENT_SECRETS_ENCRYPTION_KEY", testEncryptionKey)
	setMailEnv(t)
}

func setMailEnv(t *testing.T) {
	t.Helper()
	t.Setenv("OAGENT_MAIL_DRIVER", "smtp")
	t.Setenv("OAGENT_MAIL_FROM", testMailFrom)
	t.Setenv("OAGENT_MAIL_SMTP_HOST", "smtp.example.com")
	t.Setenv("OAGENT_HTTP_BASE_URL", "https://oagent.example.com")
}

func validConfig(t *testing.T) *Config {
	t.Helper()
	setRequiredEnv(t)
	c, err := Load(missingPath(t))
	require.NoError(t, err)
	return c
}

func TestLoad_AuthDefaults(t *testing.T) {
	c := validConfig(t)
	assert.Equal(t, AuthModeLocal, c.Auth.Mode)
	assert.Equal(t, 24*time.Hour, c.Auth.SessionTTL)
	assert.True(t, c.Auth.RememberLastUser)
	assert.Empty(t, c.Install.Token)
	assert.True(t, c.HTTP.CookieSecure)
	assert.Equal(t, testStateSecret, c.HTTP.StateSecret)
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
				c.Auth.Basic = BasicConfig{User: "ops", Password: "s3cret-pass-phrase"}
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
				c.Auth.Basic = BasicConfig{User: "ops", Password: "s3cret-pass-phrase"}
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
			mutate: func(c *Config) {
				c.Auth.Genesis = GenesisConfig{Email: "root@example.com", Password: "long-enough-phrase"}
			},
			check: func(t *testing.T, err error) { assert.NoError(t, err) },
		},
		"genesis in cloud": {
			mutate: func(c *Config) {
				oidcOn(c)
				c.Mode = ModeCloud
				c.Auth.Genesis = GenesisConfig{Email: "root@example.com", Password: "long-enough-phrase"}
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
		"production local without state secret": {
			mutate: func(c *Config) { c.HTTP.StateSecret = "" },
			check: func(t *testing.T, err error) {
				assert.True(t, IsMissingSettingError(err), "got %v", err)
				assert.Contains(t, err.Error(), "http.stateSecret")
				assert.Contains(t, err.Error(), "openssl rand -hex 32")
			},
		},
		"short state secret": {
			mutate: func(c *Config) { c.HTTP.StateSecret = "too-short" },
			check: func(t *testing.T, err error) {
				assert.True(t, IsInvalidSettingError(err), "got %v", err)
				assert.Contains(t, err.Error(), "openssl rand -hex 32")
			},
		},
		"production local without encryption key": {
			mutate: func(c *Config) { c.Secrets.EncryptionKey = "" },
			check: func(t *testing.T, err error) {
				assert.True(t, IsMissingSettingError(err), "got %v", err)
				assert.Contains(t, err.Error(), "secrets.encryptionKey")
				assert.Contains(t, err.Error(), "openssl rand -hex 32")
			},
		},
		"production local with a bad encryption key": {
			mutate: func(c *Config) { c.Secrets.EncryptionKey = "abcd" },
			check:  func(t *testing.T, err error) { assert.True(t, IsInvalidSettingError(err), "got %v", err) },
		},
		"production basic needs no state secret": {
			mutate: func(c *Config) {
				c.Auth.Mode = AuthModeBasic
				c.Auth.Basic = BasicConfig{User: "ops", Password: "s3cret-pass-phrase"}
				c.HTTP.StateSecret = ""
			},
			check: func(t *testing.T, err error) { assert.NoError(t, err) },
		},
		"development with console mail needs no secrets": {
			mutate: func(c *Config) {
				c.Env = EnvDevelopment
				c.Mail.Driver = MailDriverConsole
				c.HTTP.StateSecret, c.Secrets.EncryptionKey = "", ""
			},
			check: func(t *testing.T, err error) { assert.NoError(t, err) },
		},
		"insecure cookie in production": {
			mutate: func(c *Config) { c.HTTP.CookieSecure = false },
			check:  func(t *testing.T, err error) { assert.NoError(t, err) },
		},
		"insecure cookie in development": {
			mutate: func(c *Config) { c.Env, c.HTTP.CookieSecure = EnvDevelopment, false },
			check:  func(t *testing.T, err error) { assert.NoError(t, err) },
		},
		"short install token": {
			mutate: func(c *Config) { c.Env, c.Install.Token = EnvDevelopment, "too-short" },
			check: func(t *testing.T, err error) {
				assert.True(t, IsInvalidSettingError(err), "got %v", err)
				assert.Contains(t, err.Error(), "install.token")
				assert.Contains(t, err.Error(), "openssl rand -hex 32")
			},
		},
		"install token ok": {
			mutate: func(c *Config) { c.Install.Token = strings.Repeat("a", 32) },
			check:  func(t *testing.T, err error) { assert.NoError(t, err) },
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

func TestValidate_ConfigPasswordRule(t *testing.T) {
	for name, tc := range map[string]struct {
		mutate func(c *Config)
		plain  string
		want   password.Reason
	}{
		"basic 11 characters": {
			mutate: func(c *Config) {
				c.Auth.Mode, c.Auth.Basic = AuthModeBasic, BasicConfig{User: "ops", Password: "tea-for-two"}
			},
			plain: "tea-for-two",
			want:  password.ReasonTooShort,
		},
		"basic contains the user": {
			mutate: func(c *Config) {
				c.Auth.Mode, c.Auth.Basic = AuthModeBasic, BasicConfig{User: "operator", Password: "the-operator-2026"}
			},
			plain: "the-operator-2026",
			want:  password.ReasonContainsName,
		},
		"genesis contains the email": {
			mutate: func(c *Config) { c.Auth.Genesis = GenesisConfig{Email: "root@example.com", Password: "i-am-root-2026"} },
			plain:  "i-am-root-2026",
			want:   password.ReasonContainsEmail,
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := validConfig(t)
			tc.mutate(c)
			err := c.validateAuth()
			pe, ok := errors.AsType[*password.PolicyError](err)
			require.True(t, ok, "got %v", err)
			assert.Equal(t, tc.want, pe.Reason)
			assert.NotContains(t, err.Error(), tc.plain)
		})
	}
}

func TestValidate_StrongConfigPasswordsPass(t *testing.T) {
	c := validConfig(t)
	c.Auth.Mode, c.Auth.Basic = AuthModeBasic, BasicConfig{User: "operator", Password: "correct horse battery"}
	c.Auth.Genesis = GenesisConfig{Email: "root@example.com", Password: "correct horse battery"}
	assert.NoError(t, c.validateAuth())
}

func TestErrors_Messages(t *testing.T) {
	assert.Equal(t, `config: auth.mode "basic" is not allowed with mode "cloud"`, (&AuthModeError{AuthMode: AuthModeBasic, Mode: ModeCloud}).Error())
	assert.Equal(t, "config: auth.basic.user required: auth.mode is basic", (&MissingSettingError{Keys: "auth.basic.user", Reason: "auth.mode is basic"}).Error())
	assert.Contains(t, (&GenesisInCloudError{}).Error(), "selfhosted only")
	assert.Equal(t, "config: http.stateSecret: too short", (&InvalidSettingError{Key: "http.stateSecret", Reason: "too short"}).Error())
}

// auth.password.enabled: unset is on in selfhosted and off in cloud; cloud refuses it on.
func TestPasswordsOn(t *testing.T) {
	c := validConfig(t)
	assert.True(t, c.PasswordsOn(), "selfhosted default")
	c.Auth.Password.Enabled = new(false)
	assert.False(t, c.PasswordsOn())
	assert.False(t, c.OpenSignup(), "selfhosted joins by invite only")
	oidcOn(c)
	c.Mode, c.Auth.Password.Enabled = ModeCloud, nil
	assert.True(t, c.OpenSignup())
	assert.False(t, c.PasswordsOn(), "cloud default")
	c.Auth.Password.Enabled = new(true)
	var invalid *InvalidSettingError
	require.ErrorAs(t, c.validateAuth(), &invalid)
	assert.Equal(t, "auth.password.enabled", invalid.Key)
}

func TestValidate_GenesisFollowsPasswords(t *testing.T) {
	for name, tc := range map[string]struct {
		mutate func(c *Config)
		check  func(t *testing.T, err error)
	}{
		"passwords off: email alone installs": {
			mutate: func(c *Config) {
				c.Auth.Password.Enabled, c.Auth.Genesis = new(false), GenesisConfig{Email: "root@example.com"}
			},
			check: func(t *testing.T, err error) { assert.NoError(t, err) },
		},
		"passwords off: a password is refused": {
			mutate: func(c *Config) {
				c.Auth.Password.Enabled, c.Auth.Genesis = new(false), GenesisConfig{Email: "root@example.com", Password: "correct horse battery"}
			},
			check: func(t *testing.T, err error) {
				var invalid *InvalidSettingError
				require.ErrorAs(t, err, &invalid)
				assert.Equal(t, "auth.genesis.password", invalid.Key)
			},
		},
		"passwords off: a password alone is refused": {
			mutate: func(c *Config) {
				c.Auth.Password.Enabled, c.Auth.Genesis = new(false), GenesisConfig{Password: "correct horse battery"}
			},
			check: func(t *testing.T, err error) {
				var invalid *InvalidSettingError
				require.ErrorAs(t, err, &invalid)
				assert.Equal(t, "auth.genesis.password", invalid.Key)
			},
		},
		"passwords on: email alone is missing the password": {
			mutate: func(c *Config) { c.Auth.Genesis = GenesisConfig{Email: "root@example.com"} },
			check: func(t *testing.T, err error) {
				var missing *MissingSettingError
				assert.ErrorAs(t, err, &missing)
			},
		},
		"cloud: genesis stays refused": {
			mutate: func(c *Config) { c.Mode, c.Auth.Genesis = ModeCloud, GenesisConfig{Email: "root@example.com"} },
			check:  func(t *testing.T, err error) { assert.True(t, IsGenesisInCloudError(err), "%v", err) },
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := validConfig(t)
			oidcOn(c)
			tc.mutate(c)
			tc.check(t, c.validateAuth())
		})
	}
}

func TestLoad_PasswordEnabledFromEnv(t *testing.T) {
	setRequiredEnv(t)
	c, err := Load(missingPath(t))
	require.NoError(t, err)
	assert.Nil(t, c.Auth.Password.Enabled, "unset: the mode decides")

	t.Setenv("OAGENT_AUTH_PASSWORD_ENABLED", "false")
	t.Setenv("OAGENT_OIDC_ISSUER", "https://idp.example.com")
	t.Setenv("OAGENT_OIDC_CLIENT_ID", "oagent")
	c, err = Load(missingPath(t))
	require.NoError(t, err)
	require.NotNil(t, c.Auth.Password.Enabled)
	assert.False(t, c.PasswordsOn())
}

// A blank auth.password.enabled stays unset, so the mode's default applies.
func TestLoad_BlankPasswordEnabledKeepsTheDefault(t *testing.T) {
	setRequiredEnv(t)
	path := filepath.Join(t.TempDir(), "oagent.yaml")
	require.NoError(t, os.WriteFile(path, []byte("auth:\n  password:\n    enabled: \"\"\n"), 0o600))
	c, err := Load(path)
	require.NoError(t, err)
	assert.Nil(t, c.Auth.Password.Enabled)
	assert.True(t, c.PasswordsOn())

	t.Setenv("OAGENT_AUTH_PASSWORD_ENABLED", "  ")
	c, err = Load(missingPath(t))
	require.NoError(t, err)
	assert.Nil(t, c.Auth.Password.Enabled)

	t.Setenv("OAGENT_AUTH_PASSWORD_ENABLED", "true")
	c, err = Load(missingPath(t))
	require.NoError(t, err)
	require.NotNil(t, c.Auth.Password.Enabled)
	assert.True(t, c.PasswordsOn())
}

func TestLoad_RememberLastUserFromEnv(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("OAGENT_AUTH_REMEMBER_LAST_USER", "false")
	c, err := Load(missingPath(t))
	require.NoError(t, err)
	assert.False(t, c.Auth.RememberLastUser)
}
