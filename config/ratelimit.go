package config

// RateLimitStore selects where rate limit counters live.
type RateLimitStore string

// The rate limit stores: memory counts per server, redis shares the counts across servers.
const (
	RateLimitStoreMemory RateLimitStore = "memory"
	RateLimitStoreRedis  RateLimitStore = "redis"
)

// RateLimitConfig configures oagent's own rate limiter: one shared store, one switch per group, every group off by default.
type RateLimitConfig struct {
	Store   RateLimitStore       `yaml:"store"   mapstructure:"store"   validate:"oneof=memory redis"`
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
	if c.RateLimit.Store == RateLimitStoreMemory && c.RateLimit.MaxKeys < 1 {
		return &InvalidSettingError{Key: "rateLimit.maxKeys", Reason: "must be at least 1"}
	}
	return nil
}
