package controlplane

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
)

func TestOTel(t *testing.T) {
	ic, err := OTel(nil, nil)
	require.NoError(t, err)
	assert.NotNil(t, ic)

	ic, err = OTel(nooptrace.NewTracerProvider(), noopmetric.NewMeterProvider())
	require.NoError(t, err)
	assert.NotNil(t, ic)
}
