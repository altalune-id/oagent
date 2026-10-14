package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_MailDefaults(t *testing.T) {
	setSecretEnv(t)
	t.Setenv("OAGENT_POSTGRES_DSN", "postgres://localhost/db")
	t.Setenv("OAGENT_ENV", "development")
	t.Setenv("OAGENT_MAIL_DRIVER", "")

	c, err := Load(missingPath(t))
	require.NoError(t, err)
	assert.Equal(t, MailDriverConsole, c.Mail.Driver)
	assert.Equal(t, 587, c.Mail.SMTP.Port)
	assert.Equal(t, MailTLSStartTLS, c.Mail.SMTP.TLS)
	assert.Equal(t, 10*time.Second, c.Mail.SMTP.Timeout)
}

func TestLoad_MailFromEnv(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("OAGENT_MAIL_DRIVER", "resend")
	t.Setenv("OAGENT_MAIL_RESEND_API_KEY", "re_test")
	c, err := Load(missingPath(t))
	require.NoError(t, err)
	assert.Equal(t, MailDriverResend, c.Mail.Driver)
	assert.Equal(t, "re_test", c.Mail.Resend.APIKey)
	assert.Equal(t, testMailFrom, c.Mail.From)
}

func TestValidate_Mail(t *testing.T) {
	for name, tc := range map[string]struct {
		mutate func(c *Config)
		check  func(t *testing.T, err error)
	}{
		"smtp in production ok": {
			mutate: func(*Config) {},
			check:  func(t *testing.T, err error) { assert.NoError(t, err) },
		},
		"resend in production ok": {
			mutate: func(c *Config) {
				c.Mail.Driver = MailDriverResend
				c.Mail.Resend.APIKey = "re_test"
			},
			check: func(t *testing.T, err error) { assert.NoError(t, err) },
		},
		"console in development ok without from": {
			mutate: func(c *Config) {
				c.Env = EnvDevelopment
				c.Mail = MailConfig{Driver: MailDriverConsole}
				c.HTTP.BaseURL = ""
			},
			check: func(t *testing.T, err error) { assert.NoError(t, err) },
		},
		"unknown driver": {
			mutate: func(c *Config) { c.Mail.Driver = "pigeon" },
			check:  invalidSetting("mail.driver"),
		},
		"console in production": {
			mutate: func(c *Config) { c.Mail.Driver = MailDriverConsole },
			check:  invalidSetting("mail.driver"),
		},
		"tls none in production": {
			mutate: func(c *Config) { c.Mail.SMTP.TLS = MailTLSNone },
			check:  invalidSetting("mail.smtp.tls"),
		},
		"tls none in development ok": {
			mutate: func(c *Config) {
				c.Env = EnvDevelopment
				c.Mail.SMTP.TLS = MailTLSNone
			},
			check: func(t *testing.T, err error) { assert.NoError(t, err) },
		},
		"unknown tls": {
			mutate: func(c *Config) { c.Mail.SMTP.TLS = "ssl" },
			check:  invalidSetting("mail.smtp.tls"),
		},
		"missing from with real driver": {
			mutate: func(c *Config) { c.Mail.From = "" },
			check:  missingSetting("mail.from"),
		},
		"unparsable from": {
			mutate: func(c *Config) { c.Mail.From = "not an address" },
			check:  invalidSetting("mail.from"),
		},
		"unparsable from with console": {
			mutate: func(c *Config) {
				c.Env = EnvDevelopment
				c.Mail.Driver = MailDriverConsole
				c.Mail.From = "nope"
			},
			check: invalidSetting("mail.from"),
		},
		"missing base url with real driver": {
			mutate: func(c *Config) { c.HTTP.BaseURL = "" },
			check:  missingSetting("http.baseURL"),
		},
		"smtp without host": {
			mutate: func(c *Config) { c.Mail.SMTP.Host = "" },
			check:  missingSetting("mail.smtp.host"),
		},
		"smtp bad port": {
			mutate: func(c *Config) { c.Mail.SMTP.Port = 0 },
			check:  invalidSetting("mail.smtp.port"),
		},
		"smtp bad timeout": {
			mutate: func(c *Config) { c.Mail.SMTP.Timeout = 0 },
			check:  invalidSetting("mail.smtp.timeout"),
		},
		"smtp timeout longer than a claim": {
			mutate: func(c *Config) { c.Mail.SMTP.Timeout = MaxSMTPTimeout + time.Second },
			check:  invalidSetting("mail.smtp.timeout"),
		},
		"smtp in development without encryption key": {
			mutate: func(c *Config) {
				c.Env = EnvDevelopment
				c.Secrets.EncryptionKey = ""
			},
			check: missingSetting("secrets.encryptionKey"),
		},
		"resend in development with a bad encryption key": {
			mutate: func(c *Config) {
				c.Env = EnvDevelopment
				c.Mail.Driver = MailDriverResend
				c.Mail.Resend.APIKey = "re_test"
				c.Secrets.EncryptionKey = "short"
			},
			check: invalidSetting("secrets.encryptionKey"),
		},
		"smtp with basic auth without encryption key": {
			mutate: func(c *Config) {
				c.Auth.Mode = AuthModeBasic
				c.Auth.Basic = BasicConfig{User: "ops", Password: "s3cret-pass-phrase"}
				c.Secrets.EncryptionKey = ""
			},
			check: missingSetting("secrets.encryptionKey"),
		},
		"console in development ok without encryption key": {
			mutate: func(c *Config) {
				c.Env = EnvDevelopment
				c.Mail = MailConfig{Driver: MailDriverConsole}
				c.Secrets.EncryptionKey = ""
			},
			check: func(t *testing.T, err error) { assert.NoError(t, err) },
		},
		"resend without api key": {
			mutate: func(c *Config) { c.Mail.Driver = MailDriverResend },
			check:  missingSetting("mail.resend.apiKey"),
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := validConfig(t)
			tc.mutate(c)
			tc.check(t, c.Validate())
		})
	}
}

func TestLoad_ConsoleMailRefusedInProduction(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("OAGENT_MAIL_DRIVER", "console")
	_, err := Load(missingPath(t))
	invalidSetting("mail.driver")(t, err)
}

func invalidSetting(key string) func(t *testing.T, err error) {
	return func(t *testing.T, err error) {
		t.Helper()
		var target *InvalidSettingError
		require.ErrorAs(t, err, &target)
		assert.Equal(t, key, target.Key)
	}
}

func missingSetting(keys string) func(t *testing.T, err error) {
	return func(t *testing.T, err error) {
		t.Helper()
		var target *MissingSettingError
		require.ErrorAs(t, err, &target)
		assert.Equal(t, keys, target.Keys)
	}
}
