package boot

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	noopmetric "go.opentelemetry.io/otel/metric/noop"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/kernel"
	"altalune.id/oagent/internal/kernel/session"
)

func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestBuildScheduler(t *testing.T) {
	k := kernel.Kernel{Meter: noopmetric.NewMeterProvider().Meter("test"), Reporter: apperror.NewReporter(nil, false)}

	r, err := buildScheduler(k, config.Config{Scheduler: config.SchedulerConfig{Enabled: true}}, &Services{Sessions: session.NewMemoryStore()}, testLogger())
	require.NoError(t, err)
	require.Len(t, r.Jobs(), 1)
	assert.Equal(t, "session-sweep", r.Jobs()[0].Name)
}

func TestSchedulerConfig(t *testing.T) {
	opts := schedulerConfig(config.SchedulerConfig{ShutdownGrace: 5 * time.Second}, kernel.Kernel{}, nil)
	assert.NotNil(t, opts.Logger)
	assert.Equal(t, 5*time.Second, opts.ShutdownGrace)
	assert.NotNil(t, opts.Reporter)
}

func TestSchedulerOff(t *testing.T) {
	assert.Equal(t, "", schedulerOff(newOptions(), config.Config{Scheduler: config.SchedulerConfig{Enabled: true}}))
	assert.Equal(t, "WithScheduler(false)", schedulerOff(&options{scheduler: false}, config.Config{Scheduler: config.SchedulerConfig{Enabled: true}}))
	assert.Equal(t, "scheduler.enabled=false", schedulerOff(newOptions(), config.Config{}))
	assert.Equal(t, "WithConsumerOnly", schedulerOff(&options{scheduler: true, consumerOnly: true}, config.Config{Scheduler: config.SchedulerConfig{Enabled: true}}))
}

func TestAlreadyReported(t *testing.T) {
	assert.False(t, alreadyReported(nil))
	assert.False(t, alreadyReported(errors.New("plain")))
	assert.True(t, alreadyReported(apperror.New(apperror.CodeUnexpected, "x")))
	assert.True(t, alreadyReported(errors.Join(
		apperror.New(apperror.CodeUnexpected, "a"),
		apperror.New(apperror.CodeUnexpected, "b"),
	)))
	assert.False(t, alreadyReported(errors.Join(errors.New("plain"), apperror.New(apperror.CodeUnexpected, "a"))))
}

func TestReporterAdapter(t *testing.T) {
	reported := false
	a := reporterAdapter{
		log: testLogger(),
		report: func(context.Context, string, error, ...any) *apperror.AppError {
			reported = true
			return apperror.New(apperror.CodeUnexpected, "reported")
		},
	}

	// already an AppError → logged only, not re-reported
	a.Report(context.Background(), "m", apperror.New(apperror.CodeUnexpected, "x"))
	assert.False(t, reported)

	a.Report(context.Background(), "m", errors.New("plain"))
	assert.True(t, reported)
}
