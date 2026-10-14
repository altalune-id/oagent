package boot

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"altalune.id/oagent/config"
	"altalune.id/oagent/reqid"
	"altalune.id/oagent/telemetry"
)

type incidentCounter struct {
	counter metric.Int64Counter
}

func newIncidentCounter(mp metric.MeterProvider) *incidentCounter {
	return &incidentCounter{counter: telemetry.NewCounter(mp, "incidents.total")}
}

func (i *incidentCounter) Inc(ctx context.Context, code string) {
	i.counter.Add(ctx, 1, metric.WithAttributes(attribute.String("code", code)))
}

func requestMeta(ctx context.Context) map[string]string {
	if id := reqid.FromContext(ctx); id != "" {
		return map[string]string{"request_id": id}
	}
	return nil
}

type telemetrySetupFunc func(
	context.Context,
	telemetry.Config,
	*slog.Logger,
) (trace.TracerProvider, metric.MeterProvider, func(context.Context) error, error)

//nolint:gochecknoglobals // test seam.
var setupTelemetry telemetrySetupFunc = telemetry.Setup

func telemetryConfig(c config.TelemetryConfig) telemetry.Config {
	return telemetry.Config{
		OTLP: telemetry.OTLPConfig{
			Endpoint: c.OTLP.Endpoint,
			Protocol: c.OTLP.Protocol,
			Insecure: c.OTLP.Insecure,
			Headers:  c.OTLP.Headers,
		},
		Tracing: telemetry.TracingConfig{Enabled: c.Tracing.Enabled, SampleRatio: c.Tracing.SampleRatio},
		Metrics: telemetry.MetricsConfig{
			Enabled:        c.Metrics.Enabled,
			ExportInterval: c.Metrics.ExportInterval,
			Prometheus: telemetry.PrometheusConfig{
				Enabled: c.Metrics.Prometheus.Enabled,
				Addr:    c.Metrics.Prometheus.Addr,
				Path:    c.Metrics.Prometheus.Path,
			},
		},
	}
}
