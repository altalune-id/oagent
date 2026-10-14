package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/password"
)

func valid(t *testing.T, err error) {
	t.Helper()
	assert.NoError(t, err)
}

func devConsole(c *Config) {
	c.Env = EnvDevelopment
	c.Mail.Driver = MailDriverConsole
}

func redisFeature(c *Config) {
	c.RateLimit.Driver = RateLimitDriverRedis
	c.RateLimit.Auth.Enabled = true
	c.Redis.URL = "redis://localhost:6379/0"
}

func TestValidate_EveryRule(t *testing.T) {
	type rule struct {
		name   string
		mutate func(c *Config)
		check  func(t *testing.T, err error)
	}
	rules := []rule{
		{"mode selfhosted", func(c *Config) { c.Mode = ModeSelfhosted }, valid},
		{"mode unknown", func(c *Config) { c.Mode = "bogus" }, invalidSetting("mode")},
		{"mode empty", func(c *Config) { c.Mode = "" }, missingSetting("mode")},
		{"env development", func(c *Config) { c.Env = EnvDevelopment }, valid},
		{"env unknown", func(c *Config) { c.Env = "staging" }, invalidSetting("env")},
		{"http.addr set", func(c *Config) { c.HTTP.Addr = ":1" }, valid},
		{"http.addr empty", func(c *Config) { c.HTTP.Addr = "" }, missingSetting("http.addr")},
		{"http.baseURL not a url", func(c *Config) { c.HTTP.BaseURL = "not a url" }, invalidSetting("http.baseURL")},
		{"http.baseURL empty with smtp", func(c *Config) { c.HTTP.BaseURL = "" }, missingSetting("http.baseURL")},
		{"http.baseURL empty with console", func(c *Config) { devConsole(c); c.HTTP.BaseURL = "" }, valid},
		{"http.trustedProxyHops 2", func(c *Config) { c.HTTP.TrustedProxyHops = 2 }, valid},
		{"http.trustedProxyHops negative", func(c *Config) { c.HTTP.TrustedProxyHops = -1 }, invalidSetting("http.trustedProxyHops")},
		{"http.stateSecret short", func(c *Config) { c.HTTP.StateSecret = "short" }, invalidSetting("http.stateSecret")},
		{"http.stateSecret empty in production", func(c *Config) { c.HTTP.StateSecret = "" }, missingSetting("http.stateSecret")},
		{"http.stateSecret empty in development", func(c *Config) { c.Env = EnvDevelopment; c.HTTP.StateSecret = "" }, valid},
		{"install.token 32", func(c *Config) { c.Install.Token = testStateSecret[:32] }, valid},
		{"install.token short", func(c *Config) { c.Install.Token = testStateSecret[:31] }, invalidSetting("install.token")},
		{"postgres.dsn empty", func(c *Config) { c.Postgres.DSN = "" }, missingSetting("postgres.dsn")},
		{"postgres.maxOpenConns 0", func(c *Config) { c.Postgres.MaxOpenConns = 0 }, valid},
		{"postgres.maxOpenConns negative", func(c *Config) { c.Postgres.MaxOpenConns = -1 }, invalidSetting("postgres.maxOpenConns")},
		{"postgres.maxIdleConns negative", func(c *Config) { c.Postgres.MaxIdleConns = -1 }, invalidSetting("postgres.maxIdleConns")},
		{"postgres.reader.maxOpenConns negative", func(c *Config) { c.Postgres.Reader.MaxOpenConns = -1 }, invalidSetting("postgres.reader.maxOpenConns")},
		{"postgres.reader.maxIdleConns negative", func(c *Config) { c.Postgres.Reader.MaxIdleConns = -1 }, invalidSetting("postgres.reader.maxIdleConns")},
		{"postgres.health.interval negative", func(c *Config) { c.Postgres.Health.Interval = -time.Second }, invalidSetting("postgres.health.interval")},
		{"postgres.health.timeout negative", func(c *Config) { c.Postgres.Health.Timeout = -time.Second }, invalidSetting("postgres.health.timeout")},
		{"scheduler.shutdownGrace negative", func(c *Config) { c.Scheduler.ShutdownGrace = -time.Second }, invalidSetting("scheduler.shutdownGrace")},
		{"nats.connectTimeout negative", func(c *Config) { c.NATS.ConnectTimeout = -time.Second }, invalidSetting("nats.connectTimeout")},
		{"telemetry.otlp.protocol grpc", func(c *Config) { c.Telemetry.OTLP.Protocol = "grpc" }, valid},
		{"telemetry.otlp.protocol unknown", func(c *Config) { c.Telemetry.OTLP.Protocol = "udp" }, invalidSetting("telemetry.otlp.protocol")},
		{"telemetry.tracing.sampleRatio 1", func(c *Config) { c.Telemetry.Tracing.SampleRatio = 1 }, valid},
		{"telemetry.tracing.sampleRatio above 1", func(c *Config) { c.Telemetry.Tracing.SampleRatio = 1.5 }, invalidSetting("telemetry.tracing.sampleRatio")},
		{"telemetry.tracing.sampleRatio negative", func(c *Config) { c.Telemetry.Tracing.SampleRatio = -0.1 }, invalidSetting("telemetry.tracing.sampleRatio")},
		{"compliance.termsURL", func(c *Config) { c.Compliance.TermsURL = "https://example.com/terms" }, valid},
		{"compliance.termsURL not a url", func(c *Config) { c.Compliance.TermsURL = "terms" }, invalidSetting("compliance.termsURL")},
		{"compliance.privacyURL not a url", func(c *Config) { c.Compliance.PrivacyURL = "privacy" }, invalidSetting("compliance.privacyURL")},
		{"compliance.termsUpdatedAt past", func(c *Config) { c.Compliance.TermsUpdatedAt = "2020-01-01" }, valid},
		{"compliance.termsUpdatedAt not a date", func(c *Config) { c.Compliance.TermsUpdatedAt = "2020-13-01" }, invalidSetting("compliance.termsUpdatedAt")},
		{"compliance.termsUpdatedAt future", func(c *Config) { c.Compliance.TermsUpdatedAt = "2999-01-01" }, invalidSetting("compliance.termsUpdatedAt")},
		{"auth.mode unknown", func(c *Config) { c.Auth.Mode = "oidc" }, invalidSetting("auth.mode")},
		{"auth.mode empty", func(c *Config) { c.Auth.Mode = "" }, missingSetting("auth.mode")},
		{"auth.sessionTTL 1h", func(c *Config) { c.Auth.SessionTTL = time.Hour }, valid},
		{"auth.sessionTTL 720h", func(c *Config) { c.Auth.SessionTTL = 720 * time.Hour }, valid},
		{"auth.sessionTTL short", func(c *Config) { c.Auth.SessionTTL = 30 * time.Minute }, invalidSetting("auth.sessionTTL")},
		{"auth.sessionTTL long", func(c *Config) { c.Auth.SessionTTL = 721 * time.Hour }, invalidSetting("auth.sessionTTL")},
		{"auth.basic missing", func(c *Config) { c.Auth.Mode = AuthModeBasic }, missingSetting("auth.basic.user, auth.basic.password")},
		{"auth.basic weak password", func(c *Config) {
			c.Auth.Mode = AuthModeBasic
			c.Auth.Basic = BasicConfig{User: "ops", Password: "short"}
		}, func(t *testing.T, err error) { assert.True(t, password.IsPolicyError(err), "got %v", err) }},
		{"auth.basic in cloud", func(c *Config) {
			c.Mode = ModeCloud
			c.Auth.Mode = AuthModeBasic
			c.Auth.Basic = BasicConfig{User: "ops", Password: "s3cret-pass-phrase"}
		}, func(t *testing.T, err error) { assert.True(t, IsAuthModeError(err), "got %v", err) }},
		{"auth.genesis.email not an email", func(c *Config) {
			c.Auth.Genesis = GenesisConfig{Email: "root", Password: "long-enough-phrase"}
		}, invalidSetting("auth.genesis.email")},
		{"auth.genesis half", func(c *Config) { c.Auth.Genesis.Email = "root@example.com" }, missingSetting("auth.genesis.email, auth.genesis.password")},
		{"secrets.encryptionKey empty in production", func(c *Config) { c.Secrets.EncryptionKey = "" }, missingSetting("secrets.encryptionKey")},
		{"secrets.encryptionKey not a key", func(c *Config) { c.Secrets.EncryptionKey = "zz" }, invalidSetting("secrets.encryptionKey")},
		{"secrets.encryptionKey not a key with console in development", func(c *Config) {
			devConsole(c)
			c.Secrets.EncryptionKey = "zz"
		}, valid},
		{"mail.driver unknown", func(c *Config) { c.Mail.Driver = "fax" }, invalidSetting("mail.driver")},
		{"mail.driver empty", func(c *Config) { c.Mail.Driver = "" }, invalidSetting("mail.driver")},
		{"mail.driver console in production", func(c *Config) { c.Mail.Driver = MailDriverConsole }, invalidSetting("mail.driver")},
		{"mail.driver console in development", devConsole, valid},
		{"mail.from not an address", func(c *Config) { c.Mail.From = "not an address" }, invalidSetting("mail.from")},
		{"mail.from not an address with console", func(c *Config) { devConsole(c); c.Mail.From = "nope" }, invalidSetting("mail.from")},
		{"mail.from empty with smtp", func(c *Config) { c.Mail.From = "" }, missingSetting("mail.from")},
		{"mail.from empty with console", func(c *Config) { devConsole(c); c.Mail.From = "" }, valid},
		{"mail.smtp.host empty", func(c *Config) { c.Mail.SMTP.Host = "" }, missingSetting("mail.smtp.host")},
		{"mail.smtp.port 25", func(c *Config) { c.Mail.SMTP.Port = 25 }, valid},
		{"mail.smtp.port 0", func(c *Config) { c.Mail.SMTP.Port = 0 }, invalidSetting("mail.smtp.port")},
		{"mail.smtp.port 65536", func(c *Config) { c.Mail.SMTP.Port = 65536 }, invalidSetting("mail.smtp.port")},
		{"mail.smtp.port 0 with console", func(c *Config) { devConsole(c); c.Mail.SMTP.Port = 0 }, valid},
		{"mail.smtp.timeout 2m", func(c *Config) { c.Mail.SMTP.Timeout = 2 * time.Minute }, valid},
		{"mail.smtp.timeout 0", func(c *Config) { c.Mail.SMTP.Timeout = 0 }, invalidSetting("mail.smtp.timeout")},
		{"mail.smtp.timeout 3m", func(c *Config) { c.Mail.SMTP.Timeout = 3 * time.Minute }, invalidSetting("mail.smtp.timeout")},
		{"mail.smtp.tls implicit", func(c *Config) { c.Mail.SMTP.TLS = MailTLSImplicit }, valid},
		{"mail.smtp.tls unknown", func(c *Config) { c.Mail.SMTP.TLS = "ssl" }, invalidSetting("mail.smtp.tls")},
		{"mail.smtp.tls none in production", func(c *Config) { c.Mail.SMTP.TLS = MailTLSNone }, invalidSetting("mail.smtp.tls")},
		{"mail.smtp.tls none in development", func(c *Config) { c.Env = EnvDevelopment; c.Mail.SMTP.TLS = MailTLSNone }, valid},
		{"mail.smtp.tls unknown with console", func(c *Config) { devConsole(c); c.Mail.SMTP.TLS = "ssl" }, valid},
		{"mail.resend.apiKey empty", func(c *Config) { c.Mail.Driver = MailDriverResend }, missingSetting("mail.resend.apiKey")},
		{"rateLimit.driver unknown", func(c *Config) { c.RateLimit.Driver = "disk" }, invalidSetting("rateLimit.driver")},
		{"rateLimit.maxKeys 0 with memory", func(c *Config) { c.RateLimit.MaxKeys = 0 }, invalidSetting("rateLimit.maxKeys")},
		{"rateLimit.maxKeys 0 with redis", func(c *Config) { c.RateLimit.Driver = RateLimitDriverRedis; c.RateLimit.MaxKeys = 0 }, valid},
		{"redis used", redisFeature, valid},
		{"redis.url empty when used", func(c *Config) { redisFeature(c); c.Redis.URL = "" }, missingSetting("redis.url")},
		{"redis.url wrong scheme when used", func(c *Config) { redisFeature(c); c.Redis.URL = "http://localhost" }, invalidSetting("redis.url")},
		{"redis.url wrong scheme when unused", func(c *Config) { c.Redis.URL = "http://localhost" }, valid},
		{"redis.keyPrefix bad when used", func(c *Config) { redisFeature(c); c.Redis.KeyPrefix = "a b" }, invalidSetting("redis.keyPrefix")},
		{"redis.maxConnections 0 when used", func(c *Config) { redisFeature(c); c.Redis.MaxConnections = 0 }, invalidSetting("redis.maxConnections")},
		{"redis.maxConnections 0 when unused", func(c *Config) { c.Redis.MaxConnections = 0 }, valid},
		{"redis.minIdleConnections above max", func(c *Config) { redisFeature(c); c.Redis.MinIdleConnections = 11 }, invalidSetting("redis.minIdleConnections")},
		{"redis.minIdleConnections negative", func(c *Config) { redisFeature(c); c.Redis.MinIdleConnections = -1 }, invalidSetting("redis.minIdleConnections")},
		{"redis.maxIdleTime 0", func(c *Config) { redisFeature(c); c.Redis.MaxIdleTime = 0 }, invalidSetting("redis.maxIdleTime")},
		{"redis.connectTimeout 0", func(c *Config) { redisFeature(c); c.Redis.ConnectTimeout = 0 }, invalidSetting("redis.connectTimeout")},
		{"redis.readTimeout 0", func(c *Config) { redisFeature(c); c.Redis.ReadTimeout = 0 }, invalidSetting("redis.readTimeout")},
		{"redis.writeTimeout 0", func(c *Config) { redisFeature(c); c.Redis.WriteTimeout = 0 }, invalidSetting("redis.writeTimeout")},
		{"cloud requires oidc", func(c *Config) { c.Mode = ModeCloud }, func(t *testing.T, err error) { assert.True(t, IsCloudRequiresOIDCError(err), "got %v", err) }},
	}
	for _, r := range rules {
		t.Run(r.name, func(t *testing.T) {
			c := validConfig(t)
			r.mutate(c)
			r.check(t, c.Validate())
		})
	}
}

func TestValidate_DefaultsWithDevelopmentConsole(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("OAGENT_ENV", "development")
	t.Setenv("OAGENT_MAIL_DRIVER", "console")
	_, err := Load(missingPath(t))
	require.NoError(t, err)
}

func TestValidate_TagErrorsNameTheYAMLKey(t *testing.T) {
	c := validConfig(t)
	c.Postgres.Health.Timeout = -time.Second
	err := c.Validate()
	invalidSetting("postgres.health.timeout")(t, err)
	assert.Equal(t, "config: postgres.health.timeout: must be 0 or more", err.Error())
}

func TestValidate_TagReasons(t *testing.T) {
	for name, tc := range map[string]struct {
		mutate func(c *Config)
		want   string
	}{
		"required": {func(c *Config) { c.HTTP.Addr = "" }, "config: http.addr required: must not be empty"},
		"oneof":    {func(c *Config) { c.Mode = "bogus" }, "config: mode: must be one of selfhosted, cloud"},
		"min duration": {
			func(c *Config) { c.Auth.SessionTTL = time.Minute },
			"config: auth.sessionTTL: must be at least 1h",
		},
		"max duration": {
			func(c *Config) { c.Auth.SessionTTL = 721 * time.Hour },
			"config: auth.sessionTTL: must be at most 720h",
		},
		"lte number": {
			func(c *Config) { c.Telemetry.Tracing.SampleRatio = 2 },
			"config: telemetry.tracing.sampleRatio: must be at most 1",
		},
		"url":   {func(c *Config) { c.HTTP.BaseURL = "nope" }, "config: http.baseURL: must be an absolute URL"},
		"email": {func(c *Config) { c.Auth.Genesis.Email = "root" }, "config: auth.genesis.email: must be an email address"},
		"date": {
			func(c *Config) { c.Compliance.TermsUpdatedAt = "01/02/2020" },
			"config: compliance.termsUpdatedAt: must be a date as YYYY-MM-DD",
		},
		"secret": {
			func(c *Config) { c.Install.Token = "short" },
			"config: install.token: must be at least 32 characters; " + generateHint,
		},
		"mail address": {
			func(c *Config) { c.Mail.From = "nope" },
			"config: mail.from: must be an email address, optionally with a name: Name <addr@example.com>",
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := validConfig(t)
			tc.mutate(c)
			assert.EqualError(t, c.Validate(), tc.want)
		})
	}
}

func TestValidate_ReportsEveryTagError(t *testing.T) {
	c := validConfig(t)
	c.Mode = "bogus"
	c.HTTP.Addr = ""
	c.Mail.Driver = "fax"
	c.HTTP.TrustedProxyHops = -1
	err := c.Validate()
	invalidSetting("mode")(t, err)
	missingSetting("http.addr")(t, err)
	for _, key := range []string{"mode", "http.addr", "mail.driver", "http.trustedProxyHops"} {
		assert.Contains(t, err.Error(), key)
	}
}

func TestValidate_NoSecretInMessages(t *testing.T) {
	const secret = "hunter2-do-not-print"
	for name, mutate := range map[string]func(c *Config){
		"http.stateSecret":      func(c *Config) { c.HTTP.StateSecret = secret },
		"install.token":         func(c *Config) { c.Install.Token = secret },
		"secrets.encryptionKey": func(c *Config) { c.Secrets.EncryptionKey = secret },
		"redis.url":             func(c *Config) { redisFeature(c); c.Redis.URL = "redis://:" + secret + "@[bad" },
		"auth.basic.password": func(c *Config) {
			c.Auth.Mode = AuthModeBasic
			c.Auth.Basic = BasicConfig{User: "ops", Password: "hunter2"}
		},
		"mail.from": func(c *Config) { c.Mail.From = secret },
	} {
		t.Run(name, func(t *testing.T) {
			c := validConfig(t)
			mutate(c)
			err := c.Validate()
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "hunter2")
		})
	}
}

func TestCustomTags(t *testing.T) {
	for _, tc := range []struct {
		tag, value string
		ok         bool
	}{
		{"mail_address", "a@example.com", true},
		{"mail_address", "Name <a@example.com>", true},
		{"mail_address", "a", false},
		{"secret", testStateSecret[:minSecretLen], true},
		{"secret", testStateSecret[:minSecretLen-1], false},
	} {
		t.Run(tc.tag+" "+tc.value, func(t *testing.T) {
			assert.Equal(t, tc.ok, v10.Var(tc.value, tc.tag) == nil)
		})
	}
}
