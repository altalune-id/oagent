package telemetry

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfig_Validate(t *testing.T) {
	assert.NoError(t, Config{}.Validate())
	assert.Error(t, Config{OTLP: OTLPConfig{Protocol: "bogus"}}.Validate())
	assert.Error(t, Config{Tracing: TracingConfig{SampleRatio: 2}}.Validate())
	assert.NoError(t, Config{OTLP: OTLPConfig{Protocol: "grpc"}, Tracing: TracingConfig{SampleRatio: 1}}.Validate())
}
