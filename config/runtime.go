package config

// RuntimeDriver selects how Go reaches the agent runtime.
type RuntimeDriver string

// RuntimeDriverHTTP calls the runtime service over HTTP.
const RuntimeDriverHTTP RuntimeDriver = "http"

// RuntimeConfig says where the agent runtime listens; an empty URL leaves runs unavailable.
type RuntimeConfig struct {
	Driver RuntimeDriver `yaml:"driver" mapstructure:"driver" validate:"oneof=http"`
	URL    string        `yaml:"url"    mapstructure:"url"    validate:"omitempty,url"`
	Token  string        `yaml:"token"  mapstructure:"token"  awareness:"secret" validate:"omitempty,secret"`
}

func (c *Config) validateRuntime() error {
	if c.Runtime.URL != "" && c.Runtime.Token == "" {
		return &MissingSettingError{Keys: "runtime.token", Reason: "runtime.url is set"}
	}
	return nil
}
