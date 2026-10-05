package config

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"
)

// Option customizes Load.
type Option func(*loadOptions)

type loadOptions struct {
	requireFile bool
}

// WithRequireFile makes Load fail when the config file is missing.
func WithRequireFile() Option { return func(o *loadOptions) { o.requireFile = true } }

// Load resolves defaults <- YAML file <- AUTH_* env vars (last wins) into a typed Config.
func Load(path string, opts ...Option) (*Config, error) {
	o := loadOptions{}
	for _, opt := range opts {
		opt(&o)
	}

	v := viper.NewWithOptions(viper.KeyDelimiter("."))
	setDefaults(v)

	var cfg Config
	bindEnv(v, "", reflect.TypeOf(cfg))

	v.SetEnvPrefix(EnvPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	v.AutomaticEnv()

	if path != "" {
		v.SetConfigFile(path)
	} else {
		v.SetConfigName("auth-server")
		v.AddConfigPath(".")
		if home, err := os.UserHomeDir(); err == nil {
			v.AddConfigPath(home)
		}
	}

	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		switch {
		case errors.As(err, &notFound), os.IsNotExist(err):
			if o.requireFile {
				return nil, fmt.Errorf("config: file required but not found (path=%q)", path)
			}
		default:
			return nil, fmt.Errorf("config: read %q: %w", path, err)
		}
	}

	decode := viper.DecodeHook(mapstructure.ComposeDecodeHookFunc(
		mapstructure.StringToTimeDurationHookFunc(),
		mapstructure.StringToSliceHookFunc(","),
	))
	if err := v.Unmarshal(&cfg, decode); err != nil {
		return nil, fmt.Errorf("config: unmarshal: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("mode", string(ModeSelfhosted))
	v.SetDefault("env", string(EnvDevelopment))
	v.SetDefault("http.addr", ":5150")
	v.SetDefault("scheduler.enabled", true)
	v.SetDefault("scheduler.shutdownGrace", "30s")
	v.SetDefault("queue.enabled", false)
	v.SetDefault("queue.connectTimeout", "10s")
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
