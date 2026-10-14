package config

// RateLimitStore selects where rate limit counters live.
type RateLimitStore string

// The rate limit stores. Redis is reserved and refused until its driver exists.
const (
	RateLimitStoreMemory RateLimitStore = "memory"
	RateLimitStoreRedis  RateLimitStore = "redis"
)

// RateLimitConfig configures oagent's own rate limiter: one shared store, one switch per group, every group off by default.
type RateLimitConfig struct {
	Store   RateLimitStore       `yaml:"store"   mapstructure:"store"`
	MaxKeys int                  `yaml:"maxKeys" mapstructure:"maxKeys"`
	Auth    RateLimitGroupConfig `yaml:"auth"    mapstructure:"auth"`
}

// RateLimitGroupConfig switches one group of limits on.
type RateLimitGroupConfig struct {
	Enabled bool `yaml:"enabled" mapstructure:"enabled"`
}

// AnyEnabled reports whether any group is on, so a store is needed.
func (c RateLimitConfig) AnyEnabled() bool { return c.Auth.Enabled }

func (c *Config) validateRateLimit() error {
	if c.HTTP.TrustedProxyHops < 0 {
		return &InvalidSettingError{Key: "http.trustedProxyHops", Reason: "must be 0 or more"}
	}
	r := c.RateLimit
	switch r.Store {
	case RateLimitStoreMemory:
		if r.MaxKeys <= 0 {
			return &InvalidSettingError{Key: "rateLimit.maxKeys", Reason: "must be at least 1"}
		}
		return nil
	case RateLimitStoreRedis:
		return &InvalidSettingError{Key: "rateLimit.store", Reason: "redis is not supported yet; use memory"}
	default:
		return &InvalidSettingError{Key: "rateLimit.store", Reason: "must be memory"}
	}
}
