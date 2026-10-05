package telemetry

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
)

func TestNewCounter(t *testing.T) {
	assert.NotNil(t, NewCounter(noop.NewMeterProvider(), "test.counter"))
}

type errMeterProvider struct{ metric.MeterProvider }

func (errMeterProvider) Meter(string, ...metric.MeterOption) metric.Meter { return errMeter{} }

type errMeter struct{ metric.Meter }

func (errMeter) Int64Counter(string, ...metric.Int64CounterOption) (metric.Int64Counter, error) {
	return nil, errors.New("boom")
}

func TestNewCounter_ErrorFallback(t *testing.T) {
	assert.NotNil(t, NewCounter(errMeterProvider{}, "test.counter"))
}
