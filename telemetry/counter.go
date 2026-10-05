package telemetry

import (
	"log/slog"

	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
)

// NewCounter builds an Int64Counter on mp.
func NewCounter(mp metric.MeterProvider, name string) metric.Int64Counter {
	c, err := mp.Meter(scope).Int64Counter(name)
	if err != nil {
		slog.Warn("telemetry.counter", "name", name, "error", err)
		n, _ := noop.NewMeterProvider().Meter(scope).Int64Counter(name)
		return n
	}
	return c
}
