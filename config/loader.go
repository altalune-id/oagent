package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// Option customizes Load.
type Option func(*loadOptions)

type loadOptions struct {
	requireFile    bool
	skipValidation bool
}

// WithRequireFile makes Load fail when the config file is missing.
func WithRequireFile() Option { return func(o *loadOptions) { o.requireFile = true } }

// WithoutValidation skips Validate, for CLI commands that do not boot the server (e.g. remote admin).
func WithoutValidation() Option { return func(o *loadOptions) { o.skipValidation = true } }

// Loader accumulates config sources (defaults, OAGENT_* env, bound flags) and decodes them.
type Loader struct {
	v *viper.Viper
}

// NewLoader returns a Loader seeded with defaults and OAGENT_* env bindings.
func NewLoader() *Loader {
	v := viper.NewWithOptions(viper.KeyDelimiter("."))
	setDefaults(v)
	bindEnv(v, "", reflect.TypeFor[Config]())
	v.SetEnvPrefix(EnvPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	v.AutomaticEnv()
	return &Loader{v: v}
}

// BindFlag binds flag to the dotted config key; a changed flag beats env and file.
func (l *Loader) BindFlag(key string, flag *pflag.Flag) error {
	if flag == nil {
		return fmt.Errorf("config: bind %q: nil flag", key)
	}
	return l.v.BindPFlag(key, flag)
}

// Load reads the file at path (or the standard search when empty) and decodes into Config.
func (l *Loader) Load(path string, opts ...Option) (*Config, error) {
	o := loadOptions{}
	for _, opt := range opts {
		opt(&o)
	}

	file := path
	if path == "" {
		file = findConfigFile()
	}
	if file != "" {
		l.v.SetConfigFile(file)
	}

	found := false
	if file != "" {
		switch err := l.v.ReadInConfig(); {
		case err == nil:
			found = true
		case isConfigNotFound(err):
		default:
			return nil, fmt.Errorf("config: read %q: %w", file, err)
		}
	}
	if o.requireFile && !found {
		return nil, fmt.Errorf("config: file required but not found (path=%q)", path)
	}

	var cfg Config
	decode := viper.DecodeHook(mapstructure.ComposeDecodeHookFunc(
		mapstructure.StringToTimeDurationHookFunc(),
		mapstructure.StringToSliceHookFunc(","),
	))
	if err := l.v.Unmarshal(&cfg, decode); err != nil {
		return nil, fmt.Errorf("config: unmarshal: %w", err)
	}

	if o.skipValidation {
		return &cfg, nil
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Load resolves defaults <- file <- OAGENT_* env <- bound flags. An empty path searches standard locations.
func Load(path string, opts ...Option) (*Config, error) {
	return NewLoader().Load(path, opts...)
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("mode", string(ModeSelfhosted))
	v.SetDefault("env", string(EnvProduction))
	v.SetDefault("http.addr", ":5150")
	v.SetDefault("http.cookieSecure", true)
	v.SetDefault("scheduler.enabled", true)
	v.SetDefault("scheduler.shutdownGrace", "30s")
	v.SetDefault("queue.enabled", false)
	v.SetDefault("queue.connectTimeout", "10s")
	v.SetDefault("api.openapi.enabled", true)
	v.SetDefault("api.openapi.requireBasicAuth", true)
	v.SetDefault("mcp.enabled", false)
	v.SetDefault("auth.mode", string(AuthModeLocal))
	v.SetDefault("auth.sessionTTL", "24h")
	v.SetDefault("cli.sessionFile", "~/.oagent/session.json")
	v.SetDefault("db.schema", "public")
	v.SetDefault("db.autoMigrate", true)
	v.SetDefault("db.health.interval", "30s")
	v.SetDefault("db.health.timeout", "2s")
	v.SetDefault("db.connectTimeout", "5s")
	v.SetDefault("db.connectBackoff", "250ms")
	v.SetDefault("telemetry.metrics.exportInterval", "30s")
	v.SetDefault("telemetry.metrics.prometheus.addr", ":9090")
	v.SetDefault("telemetry.metrics.prometheus.path", "/metrics")
}

// findConfigFile returns the first existing config file in configCandidates, or "".
func findConfigFile() string {
	for _, p := range configCandidates() {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	return ""
}

// configCandidates lists config files in priority order: user config dir, /etc, then cwd.
func configCandidates() []string {
	var out []string
	if dir := userConfigDir(); dir != "" {
		out = append(out, filepath.Join(dir, "config.yaml"), filepath.Join(dir, "config.yml"))
	}
	if runtime.GOOS != "windows" {
		out = append(out, "/etc/oagent/config.yaml", "/etc/oagent/config.yml")
	}
	if wd, err := os.Getwd(); err == nil {
		out = append(out, filepath.Join(wd, "config.yaml"), filepath.Join(wd, "config.yml"))
	}
	return out
}

// userConfigDir returns $XDG_CONFIG_HOME/oagent, falling back to ~/.config/oagent.
func userConfigDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "oagent")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".config", "oagent")
	}
	return ""
}

func isConfigNotFound(err error) bool {
	if _, ok := errors.AsType[viper.ConfigFileNotFoundError](err); ok {
		return true
	}
	return os.IsNotExist(err)
}
