package telemetry

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTraceExporter_UnknownProtocol(t *testing.T) {
	_, err := newTraceExporter(context.Background(), OTLPConfig{Protocol: "bad"})
	assert.Error(t, err)
}

func TestNewMetricExporter_UnknownProtocol(t *testing.T) {
	_, err := newMetricExporter(context.Background(), OTLPConfig{Protocol: "bad"})
	assert.Error(t, err)
}

func TestNewTraceExporter_HTTPOptions(t *testing.T) {
	exp, err := newTraceExporter(context.Background(), OTLPConfig{Protocol: "http", Endpoint: "localhost:4318", Insecure: true, Headers: map[string]string{"x": "y"}})
	require.NoError(t, err)
	assert.NoError(t, exp.Shutdown(context.Background()))
}

func TestNewTraceExporter_GRPC(t *testing.T) {
	exp, err := newTraceExporter(context.Background(), OTLPConfig{Protocol: "grpc", Endpoint: "localhost:4317", Insecure: true, Headers: map[string]string{"x": "y"}})
	require.NoError(t, err)
	assert.NotNil(t, exp)
	assert.NoError(t, exp.Shutdown(context.Background()))
}

func TestNewMetricExporter_HTTPOptions(t *testing.T) {
	exp, err := newMetricExporter(context.Background(), OTLPConfig{Protocol: "http", Endpoint: "localhost:4318", Insecure: true, Headers: map[string]string{"x": "y"}})
	require.NoError(t, err)
	assert.NoError(t, exp.Shutdown(context.Background()))
}

func TestNewMetricExporter_GRPC(t *testing.T) {
	exp, err := newMetricExporter(context.Background(), OTLPConfig{Protocol: "grpc", Endpoint: "localhost:4317", Insecure: true, Headers: map[string]string{"x": "y"}})
	require.NoError(t, err)
	assert.NotNil(t, exp)
	assert.NoError(t, exp.Shutdown(context.Background()))
}
