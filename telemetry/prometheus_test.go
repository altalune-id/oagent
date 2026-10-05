package telemetry

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPrometheusHandler(t *testing.T) {
	assert.NotNil(t, PrometheusHandler())
}
