package config

import (
	"net/url"
	"strings"
	"time"
)

const natsMinPassword = 16

// NATSConfig is the one NATS connection; it is opened only when a feature selects driver nats.
type NATSConfig struct {
	// URL is nats://[user:password@]host:port, or several comma-separated; tls://, ws:// and wss:// work too.
	URL            string        `yaml:"url"            mapstructure:"url"            awareness:"secret"`
	Token          string        `yaml:"token"          mapstructure:"token"          awareness:"secret"`
	User           string        `yaml:"user"           mapstructure:"user"`
	Password       string        `yaml:"password"       mapstructure:"password"       awareness:"secret"`
	Name           string        `yaml:"name"           mapstructure:"name"`
	ConnectTimeout time.Duration `yaml:"connectTimeout" mapstructure:"connectTimeout" validate:"gte=0"`
}

// NATSFeatures lists the settings that select NATS; empty means oagent never connects to it.
func (c *Config) NATSFeatures() []string {
	var out []string
	if c.Queue.Enabled && c.Queue.Driver == QueueDriverNATS {
		out = append(out, "queue.driver")
	}
	return out
}

func (c *Config) validateNATS() error {
	features := c.NATSFeatures()
	if len(features) == 0 {
		return nil
	}
	n := c.NATS
	if n.URL == "" {
		return &MissingSettingError{Keys: "nats.url", Reason: strings.Join(features, ", ") + " is nats"}
	}
	if err := validateNATSURL(n.URL); err != nil {
		return err
	}
	if n.Token != "" && (n.User != "" || n.Password != "") {
		return &InvalidSettingError{
			Key:    "nats.token",
			Reason: "set nats.token for a server without accounts, or nats.user and nats.password for one account, not both",
		}
	}
	if (n.User == "") != (n.Password == "") {
		return &MissingSettingError{Keys: "nats.user, nats.password", Reason: "set both or neither"}
	}
	if n.Password != "" && len(n.Password) < natsMinPassword {
		return &InvalidSettingError{Key: "nats.password", Reason: "must be at least 16 characters"}
	}
	return nil
}

// SECURITY: the URL may hold a password, so no error repeats it or the parser's message.
func validateNATSURL(raw string) error {
	for server := range strings.SplitSeq(raw, ",") {
		u, err := url.Parse(strings.TrimSpace(server))
		if err != nil {
			return &InvalidSettingError{Key: "nats.url", Reason: "is not a valid URL"}
		}
		switch u.Scheme {
		case "nats", "tls", "ws", "wss":
		default:
			return &InvalidSettingError{Key: "nats.url", Reason: "must start with nats://, tls://, ws:// or wss://"}
		}
		if u.Hostname() == "" {
			return &InvalidSettingError{Key: "nats.url", Reason: "must name a host"}
		}
	}
	return nil
}
