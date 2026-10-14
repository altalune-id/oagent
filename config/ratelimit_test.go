package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_RateLimitDefaults(t *testing.T) {
	c := validConfig(t)
	assert.False(t, c.RateLimit.Auth.Enabled)
	assert.False(t, c.RateLimit.AnyEnabled())
	assert.Equal(t, RateLimitDriverMemory, c.RateLimit.Driver)
	assert.Equal(t, 100_000, c.RateLimit.MaxKeys)
	assert.Zero(t, c.HTTP.TrustedProxyHops)
}

func TestLoad_RateLimitFromEnv(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("OAGENT_RATE_LIMIT_AUTH_ENABLED", "true")
	t.Setenv("OAGENT_RATE_LIMIT_MAX_KEYS", "500")
	t.Setenv("OAGENT_RATE_LIMIT_DRIVER", "memory")
	t.Setenv("OAGENT_HTTP_TRUSTED_PROXY_HOPS", "2")
	c, err := Load(missingPath(t))
	require.NoError(t, err)
	assert.True(t, c.RateLimit.Auth.Enabled)
	assert.True(t, c.RateLimit.AnyEnabled())
	assert.Equal(t, 500, c.RateLimit.MaxKeys)
	assert.Equal(t, 2, c.HTTP.TrustedProxyHops)
}

func TestValidate_RateLimit(t *testing.T) {
	for name, tc := range map[string]struct {
		mutate func(c *Config)
		check  func(t *testing.T, err error)
	}{
		"enabled with memory ok": {
			mutate: func(c *Config) { c.RateLimit.Auth.Enabled = true },
			check:  func(t *testing.T, err error) { assert.NoError(t, err) },
		},
		"redis with a url ok": {
			mutate: func(c *Config) {
				c.RateLimit.Driver = RateLimitDriverRedis
				c.RateLimit.Auth.Enabled = true
				c.Redis.URL = "redis://localhost:6379/0"
			},
			check: func(t *testing.T, err error) { assert.NoError(t, err) },
		},
		"redis without a url": {
			mutate: func(c *Config) {
				c.RateLimit.Driver = RateLimitDriverRedis
				c.RateLimit.Auth.Enabled = true
			},
			check: func(t *testing.T, err error) {
				missingSetting("redis.url")(t, err)
				assert.ErrorContains(t, err, "rateLimit.driver")
			},
		},
		"redis ignores maxKeys": {
			mutate: func(c *Config) {
				c.RateLimit.Driver = RateLimitDriverRedis
				c.RateLimit.MaxKeys = 0
			},
			check: func(t *testing.T, err error) { assert.NoError(t, err) },
		},
		"unknown driver": {
			mutate: func(c *Config) { c.RateLimit.Driver = "carrier-pigeon" },
			check:  invalidSetting("rateLimit.driver"),
		},
		"memory without keys": {
			mutate: func(c *Config) { c.RateLimit.MaxKeys = 0 },
			check:  invalidSetting("rateLimit.maxKeys"),
		},
		"two trusted proxy hops ok": {
			mutate: func(c *Config) { c.HTTP.TrustedProxyHops = 2 },
			check:  func(t *testing.T, err error) { assert.NoError(t, err) },
		},
		"negative trusted proxy hops": {
			mutate: func(c *Config) { c.HTTP.TrustedProxyHops = -1 },
			check:  invalidSetting("http.trustedProxyHops"),
		},
		"memory with negative keys": {
			mutate: func(c *Config) { c.RateLimit.MaxKeys = -1 },
			check:  invalidSetting("rateLimit.maxKeys"),
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := validConfig(t)
			tc.mutate(c)
			tc.check(t, c.Validate())
		})
	}
}
