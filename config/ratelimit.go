package config

// RateLimitDriver selects where rate limit counters live.
type RateLimitDriver string

// The rate limit drivers: memory counts per server, redis shares the counts across servers.
const (
	RateLimitDriverMemory RateLimitDriver = "memory"
	RateLimitDriverRedis  RateLimitDriver = "redis"
)

// RateLimitConfig configures oagent's own rate limiter: one shared driver, one switch per group, every group off by default.
type RateLimitConfig struct {
	Driver  RateLimitDriver      `yaml:"driver"  mapstructure:"driver"  validate:"oneof=memory redis"`
	MaxKeys int                  `yaml:"maxKeys" mapstructure:"maxKeys"`
	Auth    RateLimitGroupConfig `yaml:"auth"    mapstructure:"auth"`
}

// RateLimitGroupConfig switches one group of limits on.
type RateLimitGroupConfig struct {
	Enabled bool `yaml:"enabled" mapstructure:"enabled"`
}

// AnyEnabled reports whether any group is on, so a driver is needed.
func (c RateLimitConfig) AnyEnabled() bool { return c.Auth.Enabled }

func (c *Config) validateRateLimit() error {
	if c.RateLimit.Driver == RateLimitDriverMemory && c.RateLimit.MaxKeys < 1 {
		return &InvalidSettingError{Key: "rateLimit.maxKeys", Reason: "must be at least 1"}
	}
	return nil
}
