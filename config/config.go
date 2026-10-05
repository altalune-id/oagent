// Package config loads runtime configuration from defaults, a YAML file and AUTH_* env vars.
package config

import (
	"fmt"
	"time"

	"github.com/go-playground/validator/v10"
)

// Config is the full runtime configuration.
type Config struct {
	Mode      Mode            `yaml:"mode"      mapstructure:"mode"      awareness:"required,bootstrap" validate:"required,oneof=selfhosted cloud"`
	Env       Env             `yaml:"env"       mapstructure:"env"       awareness:"required,bootstrap" validate:"required,oneof=development production"`
	HTTP      HTTPConfig      `yaml:"http"      mapstructure:"http"`
	DB        DBConfig        `yaml:"db"        mapstructure:"db"`
	Telemetry TelemetryConfig `yaml:"telemetry" mapstructure:"telemetry"`
	Scheduler SchedulerConfig `yaml:"scheduler" mapstructure:"scheduler"`
	Queue     QueueConfig     `yaml:"queue"     mapstructure:"queue"`
	API       APIConfig       `yaml:"api"       mapstructure:"api"`
}

// HTTPConfig configures the web-facing HTTP server.
type HTTPConfig struct {
	Addr     string `yaml:"addr"     mapstructure:"addr"     awareness:"required" validate:"required"`
	BasePath string `yaml:"basePath" mapstructure:"basePath"`
}

// APIConfig configures the Connect-RPC controlplane.
type APIConfig struct {
	OpenAPI OpenAPIConfig `yaml:"openapi" mapstructure:"openapi"`
}

// OpenAPIConfig configures the OpenAPI documentation endpoints.
type OpenAPIConfig struct {
	Enabled           bool   `yaml:"enabled"           mapstructure:"enabled"`
	RequireBasicAuth  bool   `yaml:"requireBasicAuth"  mapstructure:"requireBasicAuth"`
	BasicAuthUser     string `yaml:"basicAuthUser"     mapstructure:"basicAuthUser"`
	BasicAuthPassword string `yaml:"basicAuthPassword" mapstructure:"basicAuthPassword" awareness:"secret"`
}

// SchedulerConfig configures the periodic-job runner.
type SchedulerConfig struct {
	Enabled       bool          `yaml:"enabled"       mapstructure:"enabled"`
	ShutdownGrace time.Duration `yaml:"shutdownGrace" mapstructure:"shutdownGrace" validate:"gte=0"`
}

// QueueConfig configures the NATS JetStream connection.
type QueueConfig struct {
	Enabled        bool          `yaml:"enabled"        mapstructure:"enabled"`
	URL            string        `yaml:"url"            mapstructure:"url"`
	Token          string        `yaml:"token"          mapstructure:"token"          awareness:"secret"`
	ConnectTimeout time.Duration `yaml:"connectTimeout" mapstructure:"connectTimeout" validate:"gte=0"`
}

// Mode selects the deployment posture: selfhosted (single tenant) or cloud (multi-tenant).
type Mode string

const (
	ModeSelfhosted Mode = "selfhosted"
	ModeCloud      Mode = "cloud"
)

// Env is the deployment environment.
type Env string

const (
	EnvDevelopment Env = "development"
	EnvProduction  Env = "production"
)

// IsProduction reports whether the environment is production.
func (e Env) IsProduction() bool { return e == EnvProduction }

// DBConfig is the Postgres connection configuration.
type DBConfig struct {
	DSN             string         `yaml:"dsn"             mapstructure:"dsn"             awareness:"required,secret" validate:"required"`
	Schema          string         `yaml:"schema"          mapstructure:"schema"          awareness:"bootstrap"`
	TablePrefix     string         `yaml:"tablePrefix"     mapstructure:"tablePrefix"     awareness:"bootstrap"`
	Role            string         `yaml:"role"            mapstructure:"role"`
	AutoMigrate     bool           `yaml:"autoMigrate"     mapstructure:"autoMigrate"`
	Reader          ReaderConfig   `yaml:"reader"          mapstructure:"reader"`
	Migrator        MigratorConfig `yaml:"migrator"        mapstructure:"migrator"`
	Health          HealthConfig   `yaml:"health"          mapstructure:"health"`
	MaxOpenConns    int            `yaml:"maxOpenConns"    mapstructure:"maxOpenConns"    validate:"gte=0"`
	MaxIdleConns    int            `yaml:"maxIdleConns"    mapstructure:"maxIdleConns"    validate:"gte=0"`
	ConnMaxLifetime time.Duration  `yaml:"connMaxLifetime" mapstructure:"connMaxLifetime"`
	ConnMaxIdleTime time.Duration  `yaml:"connMaxIdleTime" mapstructure:"connMaxIdleTime"`
	ConnectTimeout  time.Duration  `yaml:"connectTimeout"  mapstructure:"connectTimeout"`
	ConnectBackoff  time.Duration  `yaml:"connectBackoff"  mapstructure:"connectBackoff"`
}

// ReaderConfig points reads at a replica; an empty DSN falls back to the writer.
type ReaderConfig struct {
	DSN             string        `yaml:"dsn"             mapstructure:"dsn"             awareness:"secret"`
	Role            string        `yaml:"role"            mapstructure:"role"`
	MaxOpenConns    int           `yaml:"maxOpenConns"    mapstructure:"maxOpenConns"    validate:"gte=0"`
	MaxIdleConns    int           `yaml:"maxIdleConns"    mapstructure:"maxIdleConns"    validate:"gte=0"`
	ConnMaxLifetime time.Duration `yaml:"connMaxLifetime" mapstructure:"connMaxLifetime"`
	ConnMaxIdleTime time.Duration `yaml:"connMaxIdleTime" mapstructure:"connMaxIdleTime"`
}

// MigratorConfig is a separate credential used only to run migrations.
type MigratorConfig struct {
	DSN  string `yaml:"dsn"  mapstructure:"dsn"  awareness:"secret"`
	Role string `yaml:"role" mapstructure:"role"`
}

// HealthConfig tunes the db-health worker.
type HealthConfig struct {
	Interval time.Duration `yaml:"interval" mapstructure:"interval" validate:"gte=0"`
	Timeout  time.Duration `yaml:"timeout"  mapstructure:"timeout"  validate:"gte=0"`
}

// TelemetryConfig groups OTel + Prometheus settings.
type TelemetryConfig struct {
	OTLP    OTLPConfig    `yaml:"otlp"    mapstructure:"otlp"`
	Tracing TracingConfig `yaml:"tracing" mapstructure:"tracing"`
	Metrics MetricsConfig `yaml:"metrics" mapstructure:"metrics"`
}

// OTLPConfig configures the OTLP exporter transport.
type OTLPConfig struct {
	Endpoint string            `yaml:"endpoint" mapstructure:"endpoint"`
	Protocol string            `yaml:"protocol" mapstructure:"protocol" validate:"omitempty,oneof=http grpc"`
	Insecure bool              `yaml:"insecure" mapstructure:"insecure"`
	Headers  map[string]string `yaml:"headers"  mapstructure:"headers"`
}

// TracingConfig toggles tracing and its head sample ratio.
type TracingConfig struct {
	Enabled     bool    `yaml:"enabled"     mapstructure:"enabled"`
	SampleRatio float64 `yaml:"sampleRatio" mapstructure:"sampleRatio" validate:"gte=0,lte=1"`
}

// MetricsConfig configures the meter provider and its readers.
type MetricsConfig struct {
	Enabled        bool             `yaml:"enabled"        mapstructure:"enabled"`
	ExportInterval string           `yaml:"exportInterval" mapstructure:"exportInterval"`
	Prometheus     PrometheusConfig `yaml:"prometheus"     mapstructure:"prometheus"`
}

// PrometheusConfig configures the Prometheus /metrics endpoint.
type PrometheusConfig struct {
	Enabled bool   `yaml:"enabled" mapstructure:"enabled"`
	Addr    string `yaml:"addr"    mapstructure:"addr"`
	Path    string `yaml:"path"    mapstructure:"path"`
}

//nolint:gochecknoglobals // validator instance is stateless and safe for concurrent use.
var v10 = validator.New()

// Validate rejects invalid configuration.
func (c *Config) Validate() error {
	if err := v10.Struct(c); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	return nil
}
