package telemetry

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetup_Disabled(t *testing.T) {
	tp, mp, shutdown, err := Setup(context.Background(), Config{}, nil)
	require.NoError(t, err)
	assert.NotNil(t, tp)
	assert.NotNil(t, mp)
	assert.NoError(t, shutdown(context.Background()))
}

func TestSetup_Tracing(t *testing.T) {
	cfg := Config{Tracing: TracingConfig{Enabled: true, SampleRatio: 1}}

	_, _, shutdown, err := Setup(context.Background(), cfg, nil)
	require.NoError(t, err)
	assert.NoError(t, shutdown(context.Background()))
}

func TestSetup_MetricsPrometheus(t *testing.T) {
	cfg := Config{Metrics: MetricsConfig{Enabled: true, Prometheus: PrometheusConfig{Enabled: true}}}

	_, _, shutdown, err := Setup(context.Background(), cfg, nil)
	require.NoError(t, err)
	assert.NoError(t, shutdown(context.Background()))
}

func TestSetup_MetricsOTLP(t *testing.T) {
	cfg := Config{
		OTLP:    OTLPConfig{Endpoint: "localhost:4318", Protocol: "http"},
		Metrics: MetricsConfig{Enabled: true, ExportInterval: "1s", Prometheus: PrometheusConfig{Enabled: true}},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	_, _, shutdown, err := Setup(context.Background(), cfg, log)
	require.NoError(t, err)
	// No collector reachable: shutdown's flush error is expected here.
	_ = shutdown(context.Background())
}

func TestSetup_MetricsOTLP_BadInterval(t *testing.T) {
	cfg := Config{
		OTLP:    OTLPConfig{Endpoint: "localhost:4318", Protocol: "http"},
		Metrics: MetricsConfig{Enabled: true, ExportInterval: "bogus"},
	}

	_, _, shutdown, err := Setup(context.Background(), cfg, nil)
	require.NoError(t, err)
	_ = shutdown(context.Background())
}

func TestSetup_InvalidConfig(t *testing.T) {
	cfg := Config{OTLP: OTLPConfig{Protocol: "bad"}, Tracing: TracingConfig{Enabled: true}}

	_, _, _, err := Setup(context.Background(), cfg, nil)
	assert.Error(t, err)
}
