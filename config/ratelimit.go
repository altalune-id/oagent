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
	Driver    RateLimitDriver      `yaml:"driver"  mapstructure:"driver"  validate:"oneof=memory redis"`
	MaxKeys   int                  `yaml:"maxKeys" mapstructure:"maxKeys"`
	Auth      RateLimitGroupConfig `yaml:"auth"      mapstructure:"auth"`
	DataPlane DataPlaneLimitConfig `yaml:"dataPlane" mapstructure:"dataPlane"`
	Ingest    IngestLimitConfig    `yaml:"ingest"    mapstructure:"ingest"`
}

// IngestLimitConfig switches the inbound-delivery limits on: per channel connection (a minute, with a burst) and per client IP.
type IngestLimitConfig struct {
	Enabled                bool `yaml:"enabled"                mapstructure:"enabled"`
	PerConnectionPerMinute int  `yaml:"perConnectionPerMinute" mapstructure:"perConnectionPerMinute" validate:"gte=1"`
	Burst                  int  `yaml:"burst"                  mapstructure:"burst"                  validate:"gte=1"`
	PerIPPerMinute         int  `yaml:"perIPPerMinute"         mapstructure:"perIPPerMinute"         validate:"gte=1"`
}

// RateLimitGroupConfig switches one group of limits on.
type RateLimitGroupConfig struct {
	Enabled bool `yaml:"enabled" mapstructure:"enabled"`
}

// DataPlaneLimitConfig switches the data-plane limits on and sets how many calls a minute one API key and one client IP may make.
type DataPlaneLimitConfig struct {
	Enabled         bool `yaml:"enabled"         mapstructure:"enabled"`
	PerKeyPerMinute int  `yaml:"perKeyPerMinute" mapstructure:"perKeyPerMinute" validate:"gte=1"`
	PerIPPerMinute  int  `yaml:"perIPPerMinute"  mapstructure:"perIPPerMinute"  validate:"gte=1"`
}

// AnyEnabled reports whether any group is on, so a driver is needed.
func (c RateLimitConfig) AnyEnabled() bool {
	return c.Auth.Enabled || c.DataPlane.Enabled || c.Ingest.Enabled
}

func (c *Config) validateRateLimit() error {
	if c.RateLimit.Driver == RateLimitDriverMemory && c.RateLimit.MaxKeys < 1 {
		return &InvalidSettingError{Key: "rateLimit.maxKeys", Reason: "must be at least 1"}
	}
	return nil
}
