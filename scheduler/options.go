package scheduler

import (
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/metric"
)

// Options configures a Runner. Construct with keyed fields only.
type Options struct {
	Logger        *slog.Logger
	Reporter      ErrorReporter
	Meter         metric.Meter
	Tenants       Tenants
	Locker        Locker
	ShutdownGrace time.Duration
	Now           func() time.Time
}
