// Package config loads runtime configuration from defaults, a YAML file and OAGENT_* env vars.
package config

import "time"

// Config is the full runtime configuration.
type Config struct {
	Mode       Mode             `yaml:"mode"      mapstructure:"mode"      awareness:"required,bootstrap" validate:"required,oneof=selfhosted cloud"`       //nolint:lll // struct tags are single-line
	Env        Env              `yaml:"env"       mapstructure:"env"       awareness:"required,bootstrap" validate:"required,oneof=development production"` //nolint:lll // struct tags are single-line
	HTTP       HTTPConfig       `yaml:"http"      mapstructure:"http"`
	DB         DBConfig         `yaml:"db"        mapstructure:"db"`
	Postgres   PostgresConfig   `yaml:"postgres"  mapstructure:"postgres"`
	Telemetry  TelemetryConfig  `yaml:"telemetry" mapstructure:"telemetry"`
	Scheduler  SchedulerConfig  `yaml:"scheduler" mapstructure:"scheduler"`
	Queue      QueueConfig      `yaml:"queue"     mapstructure:"queue"`
	API        APIConfig        `yaml:"api"       mapstructure:"api"`
	MCP        MCPConfig        `yaml:"mcp"       mapstructure:"mcp"`
	Secrets    SecretsConfig    `yaml:"secrets"   mapstructure:"secrets"`
	Auth       AuthConfig       `yaml:"auth"      mapstructure:"auth"`
	Install    InstallConfig    `yaml:"install"    mapstructure:"install"`
	Compliance ComplianceConfig `yaml:"compliance" mapstructure:"compliance"`
	CLI        CLIConfig        `yaml:"cli"        mapstructure:"cli"`
	Mail       MailConfig       `yaml:"mail"       mapstructure:"mail"`
	RateLimit  RateLimitConfig  `yaml:"rateLimit"  mapstructure:"rateLimit"`
	Redis      RedisConfig      `yaml:"redis"      mapstructure:"redis"`
	NATS       NATSConfig       `yaml:"nats"       mapstructure:"nats"`
}

// CLIConfig configures the oagent CLI; SessionFile may start with "~/", expanded when used.
type CLIConfig struct {
	SessionFile string `yaml:"sessionFile" mapstructure:"sessionFile"`
}

// ComplianceConfig configures the legal pages; an empty URL serves the embedded document.
type ComplianceConfig struct {
	RequireAcceptance bool   `yaml:"requireAcceptance" mapstructure:"requireAcceptance"`
	TermsURL          string `yaml:"termsURL"          mapstructure:"termsURL"          validate:"omitempty,url"`
	PrivacyURL        string `yaml:"privacyURL"        mapstructure:"privacyURL"        validate:"omitempty,url"`
	// TermsUpdatedAt is the external terms' last change, as YYYY-MM-DD.
	TermsUpdatedAt string `yaml:"termsUpdatedAt" mapstructure:"termsUpdatedAt" validate:"omitempty,datetime=2006-01-02"`
}

// TermsUpdated returns TermsUpdatedAt as a UTC date, or the zero time when it is unset.
func (c ComplianceConfig) TermsUpdated() time.Time {
	t, _ := time.Parse(time.DateOnly, c.TermsUpdatedAt)
	return t
}

// SecretsConfig configures at-rest secret sealing (webhook signing secrets).
type SecretsConfig struct {
	EncryptionKey string `yaml:"encryptionKey" mapstructure:"encryptionKey"` // 32-byte key as hex or base64
}

// MCPConfig configures the Model Context Protocol surface mounted at BasePath+"/mcp".
type MCPConfig struct {
	Enabled          bool   `yaml:"enabled"          mapstructure:"enabled"`
	AppsUI           bool   `yaml:"appsUI"           mapstructure:"appsUI"`
	Audience         string `yaml:"audience"         mapstructure:"audience"`
	AudienceOverride bool   `yaml:"audienceOverride" mapstructure:"audienceOverride"`
	ChallengeToken   string `yaml:"challengeToken"   mapstructure:"challengeToken"`
	ChallengePrefix  string `yaml:"challengePrefix"  mapstructure:"challengePrefix"`
}

// HTTPConfig configures the web-facing HTTP server.
type HTTPConfig struct {
	Addr         string `yaml:"addr"         mapstructure:"addr"         awareness:"required" validate:"required"`
	BasePath     string `yaml:"basePath"     mapstructure:"basePath"`
	BaseURL      string `yaml:"baseURL"      mapstructure:"baseURL"      validate:"omitempty,url"`
	CookieSecure bool   `yaml:"cookieSecure" mapstructure:"cookieSecure"`
	StateSecret  string `yaml:"stateSecret"  mapstructure:"stateSecret"  awareness:"secret" validate:"omitempty,secret"`
	// TrustedProxyHops is how many proxies that append to X-Forwarded-For sit in front of oagent; 0 keys on RemoteAddr.
	TrustedProxyHops int `yaml:"trustedProxyHops" mapstructure:"trustedProxyHops" validate:"gte=0"`
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

// Validate rejects invalid configuration: every tag failure at once, else the first cross-field failure.
func (c *Config) Validate() error {
	if err := c.validateTags(); err != nil {
		return err
	}
	for _, check := range []func() error{
		c.validatePostgres,
		c.validateAuth,
		c.validateCompliance,
		c.validateMail,
		c.validateRateLimit,
		c.validateRedis,
		c.validateNATS,
	} {
		if err := check(); err != nil {
			return err
		}
	}
	return nil
}

func (c *Config) validateCompliance() error {
	// SECURITY: a future date would send every person back to /welcome on every request.
	if c.Compliance.TermsUpdated().After(time.Now()) {
		return &InvalidSettingError{Key: "compliance.termsUpdatedAt", Reason: "must not be in the future"}
	}
	return nil
}
