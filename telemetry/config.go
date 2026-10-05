package telemetry

import "fmt"

// Config groups OTel + Prometheus settings.
type Config struct {
	OTLP    OTLPConfig
	Tracing TracingConfig
	Metrics MetricsConfig
}

// OTLPConfig configures the OTLP exporter transport.
type OTLPConfig struct {
	Endpoint string
	Protocol string
	Insecure bool
	Headers  map[string]string
}

// TracingConfig toggles tracing and its head sample ratio.
type TracingConfig struct {
	Enabled     bool
	SampleRatio float64
}

// MetricsConfig configures the meter provider and its readers.
type MetricsConfig struct {
	Enabled        bool
	ExportInterval string
	Prometheus     PrometheusConfig
}

// PrometheusConfig configures the Prometheus /metrics endpoint.
type PrometheusConfig struct {
	Enabled bool
	Addr    string
	Path    string
}

// Validate rejects invalid telemetry settings.
func (c Config) Validate() error {
	if c.OTLP.Protocol != "" && c.OTLP.Protocol != "http" && c.OTLP.Protocol != "grpc" {
		return fmt.Errorf("telemetry: otlp.protocol %q must be http or grpc", c.OTLP.Protocol)
	}
	if c.Tracing.SampleRatio < 0 || c.Tracing.SampleRatio > 1 {
		return fmt.Errorf("telemetry: tracing.sampleRatio %v must be in [0,1]", c.Tracing.SampleRatio)
	}
	return nil
}
