package config

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_RedisDefaults(t *testing.T) {
	c := validConfig(t)
	assert.Empty(t, c.Redis.URL)
	assert.Equal(t, "oagent:", c.Redis.KeyPrefix)
	assert.Equal(t, 10, c.Redis.MaxConnections)
	assert.Zero(t, c.Redis.MinIdleConnections)
	assert.Equal(t, 5*time.Minute, c.Redis.MaxIdleTime)
	assert.Equal(t, 5*time.Second, c.Redis.ConnectTimeout)
	assert.Equal(t, 3*time.Second, c.Redis.ReadTimeout)
	assert.Equal(t, 3*time.Second, c.Redis.WriteTimeout)
	assert.Empty(t, c.RedisFeatures(), "nothing uses Redis by default")
}

func TestLoad_RedisFromEnv(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("OAGENT_REDIS_URL", "redis://:pw@localhost:6379/2")
	t.Setenv("OAGENT_REDIS_KEY_PREFIX", "oagent:prod")
	t.Setenv("OAGENT_REDIS_MAX_CONNECTIONS", "20")
	t.Setenv("OAGENT_REDIS_READ_TIMEOUT", "500ms")
	c, err := Load(missingPath(t))
	require.NoError(t, err)
	assert.Equal(t, "redis://:pw@localhost:6379/2", c.Redis.URL)
	assert.Equal(t, "oagent:prod:", c.Redis.KeyPrefix, "a trailing colon is added")
	assert.Equal(t, 20, c.Redis.MaxConnections)
	assert.Equal(t, 500*time.Millisecond, c.Redis.ReadTimeout)
}

func TestLoad_RedisPrefixKeepsTrailingColon(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("OAGENT_REDIS_KEY_PREFIX", "oagent:staging:")
	c, err := Load(missingPath(t))
	require.NoError(t, err)
	assert.Equal(t, "oagent:staging:", c.Redis.KeyPrefix)
}

func TestNormalizeRedisKeyPrefix(t *testing.T) {
	assert.Equal(t, "oagent:", NormalizeRedisKeyPrefix("oagent"))
	assert.Equal(t, "oagent:", NormalizeRedisKeyPrefix("oagent:"))
	assert.Empty(t, NormalizeRedisKeyPrefix(""), "empty stays empty so validation rejects it")
}

func TestValidate_Redis(t *testing.T) {
	useRedis := func(c *Config) {
		c.RateLimit.Driver = RateLimitDriverRedis
		c.RateLimit.Auth.Enabled = true
		c.Redis.URL = "redis://:secret-pw@cache.internal:6379/0"
	}
	for name, tc := range map[string]struct {
		mutate func(c *Config)
		check  func(t *testing.T, err error)
	}{
		"unused redis is ignored, even when broken": {
			mutate: func(c *Config) {
				c.Redis.URL = "http://nope"
				c.Redis.KeyPrefix = "bad prefix"
				c.Redis.MaxConnections = 0
			},
			check: func(t *testing.T, err error) { assert.NoError(t, err) },
		},
		"used with a good url ok": {
			mutate: useRedis,
			check:  func(t *testing.T, err error) { assert.NoError(t, err) },
		},
		"rediss ok": {
			mutate: func(c *Config) { useRedis(c); c.Redis.URL = "rediss://user:pw@cache.internal:6380" },
			check:  func(t *testing.T, err error) { assert.NoError(t, err) },
		},
		"used without a url names the feature": {
			mutate: func(c *Config) { useRedis(c); c.Redis.URL = "" },
			check: func(t *testing.T, err error) {
				missingSetting("redis.url")(t, err)
				assert.ErrorContains(t, err, "rateLimit.driver")
			},
		},
		"wrong scheme": {
			mutate: func(c *Config) { useRedis(c); c.Redis.URL = "http://:secret-pw@cache.internal:6379" },
			check:  redisURLError,
		},
		"no host": {
			mutate: func(c *Config) { useRedis(c); c.Redis.URL = "redis://:secret-pw@" },
			check:  redisURLError,
		},
		"unparsable": {
			mutate: func(c *Config) { useRedis(c); c.Redis.URL = "redis://:secret-pw@cache internal:6379" },
			check:  redisURLError,
		},
		"prefix with a space": {
			mutate: func(c *Config) { useRedis(c); c.Redis.KeyPrefix = "o agent:" },
			check:  invalidSetting("redis.keyPrefix"),
		},
		"prefix with a wildcard": {
			mutate: func(c *Config) { useRedis(c); c.Redis.KeyPrefix = "oagent*:" },
			check:  invalidSetting("redis.keyPrefix"),
		},
		"prefix too long": {
			mutate: func(c *Config) { useRedis(c); c.Redis.KeyPrefix = strings.Repeat("a", 64) + ":" },
			check:  invalidSetting("redis.keyPrefix"),
		},
		"prefix 64 long ok": {
			mutate: func(c *Config) { useRedis(c); c.Redis.KeyPrefix = strings.Repeat("a", 63) + ":" },
			check:  func(t *testing.T, err error) { assert.NoError(t, err) },
		},
		"empty prefix": {
			mutate: func(c *Config) { useRedis(c); c.Redis.KeyPrefix = "" },
			check:  invalidSetting("redis.keyPrefix"),
		},
		"no connections": {
			mutate: func(c *Config) { useRedis(c); c.Redis.MaxConnections = 0 },
			check:  invalidSetting("redis.maxConnections"),
		},
		"negative idle connections": {
			mutate: func(c *Config) { useRedis(c); c.Redis.MinIdleConnections = -1 },
			check:  invalidSetting("redis.minIdleConnections"),
		},
		"more idle than max": {
			mutate: func(c *Config) { useRedis(c); c.Redis.MinIdleConnections = 11 },
			check:  invalidSetting("redis.minIdleConnections"),
		},
		"zero idle time": {
			mutate: func(c *Config) { useRedis(c); c.Redis.MaxIdleTime = 0 },
			check:  invalidSetting("redis.maxIdleTime"),
		},
		"zero connect timeout": {
			mutate: func(c *Config) { useRedis(c); c.Redis.ConnectTimeout = 0 },
			check:  invalidSetting("redis.connectTimeout"),
		},
		"zero read timeout": {
			mutate: func(c *Config) { useRedis(c); c.Redis.ReadTimeout = 0 },
			check:  invalidSetting("redis.readTimeout"),
		},
		"zero write timeout": {
			mutate: func(c *Config) { useRedis(c); c.Redis.WriteTimeout = 0 },
			check:  invalidSetting("redis.writeTimeout"),
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := validConfig(t)
			tc.mutate(c)
			tc.check(t, c.validateRedis())
		})
	}
}

func redisURLError(t *testing.T, err error) {
	t.Helper()
	invalidSetting("redis.url")(t, err)
	assert.NotContains(t, err.Error(), "secret-pw", "the url is a secret")
	assert.NotContains(t, err.Error(), "cache", "the url is not echoed")
}

func TestRedisFeatures(t *testing.T) {
	c := validConfig(t)
	c.RateLimit.Driver = RateLimitDriverRedis
	assert.Empty(t, c.RedisFeatures(), "a redis store with every group off needs no connection")
	c.RateLimit.Auth.Enabled = true
	assert.Equal(t, []string{"rateLimit.driver"}, c.RedisFeatures())
	c.RateLimit.Driver = RateLimitDriverMemory
	assert.Empty(t, c.RedisFeatures())
}
