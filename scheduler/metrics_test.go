package scheduler_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

	"altalune.id/oagent/scheduler"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestMetrics(t *testing.T) {
	mp := sdkmetric.NewMeterProvider()

	r, err := scheduler.New(scheduler.Options{Logger: discardLogger(), Meter: mp.Meter("scheduler-test")})
	require.NoError(t, err)

	require.NoError(t, r.Register(scheduler.Job{
		Name:     "with-metrics",
		Scope:    scheduler.ScopeSystem,
		Schedule: scheduler.MustEveryInterval(time.Minute, 0),
		Run:      func(context.Context) error { return nil },
	}))

	require.NoError(t, r.RunOnce(context.Background(), "with-metrics"))
}

func TestDailyAtString(t *testing.T) {
	s, err := scheduler.DailyAt(9, 30, time.UTC)
	require.NoError(t, err)
	require.Contains(t, s.String(), "09:30")
}
