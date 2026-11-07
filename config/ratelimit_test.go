package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_RateLimitDefaults(t *testing.T) {
	c := validConfig(t)
	assert.True(t, c.RateLimit.Auth.Enabled, "a default deploy limits sign-in, reset and invite forms")
	assert.True(t, c.RateLimit.AnyEnabled())
	assert.Equal(t, RateLimitDriverMemory, c.RateLimit.Driver)
	assert.Equal(t, 100_000, c.RateLimit.MaxKeys)
	assert.Zero(t, c.HTTP.TrustedProxyHops)
	assert.True(t, c.RateLimit.DataPlane.Enabled, "a default deploy limits the integrator API per key and per IP")
	assert.Equal(t, 600, c.RateLimit.DataPlane.PerKeyPerMinute)
	assert.Equal(t, 600, c.RateLimit.DataPlane.PerIPPerMinute)
}

func TestLoad_DataPlaneLimitFromEnv(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("OAGENT_RATE_LIMIT_AUTH_ENABLED", "false")
	t.Setenv("OAGENT_RATE_LIMIT_DATA_PLANE_ENABLED", "true")
	t.Setenv("OAGENT_RATE_LIMIT_DATA_PLANE_PER_KEY_PER_MINUTE", "120")
	t.Setenv("OAGENT_RATE_LIMIT_DATA_PLANE_PER_IP_PER_MINUTE", "60")
	c, err := Load(missingPath(t))
	require.NoError(t, err)
	assert.True(t, c.RateLimit.DataPlane.Enabled)
	assert.True(t, c.RateLimit.AnyEnabled(), "the data-plane group alone needs a driver")
	assert.Equal(t, 120, c.RateLimit.DataPlane.PerKeyPerMinute)
	assert.Equal(t, 60, c.RateLimit.DataPlane.PerIPPerMinute)
}

func TestLoad_RateLimitFromEnv(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("OAGENT_RATE_LIMIT_AUTH_ENABLED", "false")
	t.Setenv("OAGENT_RATE_LIMIT_DATA_PLANE_ENABLED", "false")
	t.Setenv("OAGENT_RATE_LIMIT_INGEST_ENABLED", "false")
	t.Setenv("OAGENT_RATE_LIMIT_MAX_KEYS", "500")
	t.Setenv("OAGENT_RATE_LIMIT_DRIVER", "memory")
	t.Setenv("OAGENT_HTTP_TRUSTED_PROXY_HOPS", "2")
	c, err := Load(missingPath(t))
	require.NoError(t, err)
	assert.False(t, c.RateLimit.Auth.Enabled, "an edge-limited deploy can turn it off")
	assert.False(t, c.RateLimit.AnyEnabled())
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
				c.RateLimit.Auth.Enabled = false
				c.RateLimit.DataPlane.Enabled = false
				c.RateLimit.Ingest.Enabled = false
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
		"data plane per key zero": {
			mutate: func(c *Config) { c.RateLimit.DataPlane.PerKeyPerMinute = 0 },
			check:  invalidSetting("rateLimit.dataPlane.perKeyPerMinute"),
		},
		"data plane per ip zero": {
			mutate: func(c *Config) { c.RateLimit.DataPlane.PerIPPerMinute = 0 },
			check:  invalidSetting("rateLimit.dataPlane.perIPPerMinute"),
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
