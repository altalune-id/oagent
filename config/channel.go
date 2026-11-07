package config

import "time"

// ChannelConfig configures messaging channels: a new connection's OpenWA URL and the oldest inbound message a turn answers.
type ChannelConfig struct {
	DefaultOpenwaBaseURL string        `yaml:"defaultOpenwaBaseURL" mapstructure:"defaultOpenwaBaseURL" validate:"omitempty,url"`
	MaxMessageAge        time.Duration `yaml:"maxMessageAge"        mapstructure:"maxMessageAge"        validate:"gte=1m"`
	// AllowPrivateHosts lets a production deploy reach OpenWA on a private network over http.
	AllowPrivateHosts bool `yaml:"allowPrivateHosts" mapstructure:"allowPrivateHosts"`
}
