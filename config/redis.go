package config

import (
	"net/url"
	"regexp"
	"strings"
	"time"
)

// RedisConfig is the one Redis connection; it is opened only when a feature selects driver redis.
type RedisConfig struct {
	// URL is redis://[user:password@]host:port[/db] or rediss:// for TLS.
	URL                string        `yaml:"url"                mapstructure:"url"                awareness:"secret"`
	KeyPrefix          string        `yaml:"keyPrefix"          mapstructure:"keyPrefix"`
	MaxConnections     int           `yaml:"maxConnections"     mapstructure:"maxConnections"`
	MinIdleConnections int           `yaml:"minIdleConnections" mapstructure:"minIdleConnections"`
	MaxIdleTime        time.Duration `yaml:"maxIdleTime"        mapstructure:"maxIdleTime"`
	ConnectTimeout     time.Duration `yaml:"connectTimeout"     mapstructure:"connectTimeout"`
	ReadTimeout        time.Duration `yaml:"readTimeout"        mapstructure:"readTimeout"`
	WriteTimeout       time.Duration `yaml:"writeTimeout"       mapstructure:"writeTimeout"`
}

//nolint:gochecknoglobals // compiled once, read-only.
var redisKeyPrefixRE = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,64}$`)

// NormalizeRedisKeyPrefix adds the trailing ":" a non-empty prefix is missing.
func NormalizeRedisKeyPrefix(p string) string {
	if p == "" || strings.HasSuffix(p, ":") {
		return p
	}
	return p + ":"
}

// RedisFeatures lists the settings that select Redis; empty means oagent never connects to it.
func (c *Config) RedisFeatures() []string {
	var out []string
	if c.RateLimit.Driver == RateLimitDriverRedis && c.RateLimit.AnyEnabled() {
		out = append(out, "rateLimit.driver")
	}
	return out
}

func (c *Config) validateRedis() error {
	features := c.RedisFeatures()
	if len(features) == 0 {
		return nil
	}
	r := c.Redis
	if r.URL == "" {
		return &MissingSettingError{Keys: "redis.url", Reason: strings.Join(features, ", ") + " is redis"}
	}
	if err := validateRedisURL(r.URL); err != nil {
		return err
	}
	if !redisKeyPrefixRE.MatchString(r.KeyPrefix) {
		return &InvalidSettingError{Key: "redis.keyPrefix", Reason: "must be 1-64 letters, digits or . _ : -"}
	}
	switch {
	case r.MaxConnections < 1:
		return &InvalidSettingError{Key: "redis.maxConnections", Reason: "must be at least 1"}
	case r.MinIdleConnections < 0 || r.MinIdleConnections > r.MaxConnections:
		return &InvalidSettingError{Key: "redis.minIdleConnections", Reason: "must be between 0 and redis.maxConnections"}
	case r.MaxIdleTime <= 0:
		return &InvalidSettingError{Key: "redis.maxIdleTime", Reason: "must be positive"}
	case r.ConnectTimeout <= 0:
		return &InvalidSettingError{Key: "redis.connectTimeout", Reason: "must be positive"}
	case r.ReadTimeout <= 0:
		return &InvalidSettingError{Key: "redis.readTimeout", Reason: "must be positive"}
	case r.WriteTimeout <= 0:
		return &InvalidSettingError{Key: "redis.writeTimeout", Reason: "must be positive"}
	}
	return nil
}

// SECURITY: the URL may hold a password, so no error repeats it or the parser's message.
func validateRedisURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return &InvalidSettingError{Key: "redis.url", Reason: "is not a valid URL"}
	}
	if u.Scheme != "redis" && u.Scheme != "rediss" {
		return &InvalidSettingError{Key: "redis.url", Reason: "must start with redis:// or rediss://"}
	}
	if u.Hostname() == "" {
		return &InvalidSettingError{Key: "redis.url", Reason: "must name a host"}
	}
	return nil
}
