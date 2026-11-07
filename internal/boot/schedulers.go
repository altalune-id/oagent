package boot

import (
	"context"
	"fmt"
	"log/slog"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/kernel"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/run"
	"altalune.id/oagent/scheduler"
	"altalune.id/oagent/worker"
)

func schedulerOff(o *options, cfg config.Config) string {
	switch {
	case !o.scheduler:
		return "WithScheduler(false)"
	case o.consumerOnly:
		return "WithConsumerOnly"
	case !cfg.Scheduler.Enabled:
		return "scheduler.enabled=false"
	}
	return ""
}

func schedulerConfig(c config.SchedulerConfig, k kernel.Kernel, log *slog.Logger) scheduler.Options {
	if log == nil {
		log = slog.Default()
	}
	return scheduler.Options{
		Logger:        log,
		Reporter:      reporterAdapter{report: k.Reporter.Unexpected, log: log},
		Meter:         k.Meter,
		ShutdownGrace: c.ShutdownGrace,
	}
}

func buildScheduler(k kernel.Kernel, cfg config.Config, s *Services, log *slog.Logger) (*scheduler.Runner, error) {
	opts := schedulerConfig(cfg.Scheduler, k, log)
	opts.Tenants = outboxTenants{orgs: s.Orgs}
	r, err := scheduler.New(opts)
	if err != nil {
		return nil, err
	}
	jobs := session.NewScheduler(s.Sessions, orDiscard(log)).SchedulerJobs()
	if s.RunStore != nil {
		jobs = append(jobs, run.Jobs(s.RunStore, s.RunKicker, cfg.Run.SweepEvery)...)
	}
	if s.Channels != nil {
		jobs = append(jobs, s.Channels.SchedulerJobs()...)
	}
	if s.ChannelReplier != nil {
		jobs = append(jobs, s.ChannelReplier.SchedulerJobs()...)
	}
	for _, j := range jobs {
		if err := r.Register(j); err != nil {
			return nil, fmt.Errorf("boot: register %s: %w", j.Name, err)
		}
	}
	return r, nil
}

var _ worker.Worker = (*scheduler.Runner)(nil)
var _ scheduler.ErrorReporter = reporterAdapter{}

type reporterAdapter struct {
	report apperror.UnexpectedFunc
	log    *slog.Logger
}

func (a reporterAdapter) Report(ctx context.Context, message string, cause error, attrs ...any) {
	if alreadyReported(cause) {
		a.log.ErrorContext(ctx, message, append([]any{slog.Any("error", cause)}, attrs...)...)
		return
	}
	_ = a.report(ctx, message, cause, attrs...)
}

func alreadyReported(err error) bool {
	if err == nil {
		return false
	}
	if _, ok := apperror.AsAppError(err); ok {
		return true
	}
	joined, ok := err.(interface{ Unwrap() []error })
	if !ok {
		return false
	}
	causes := joined.Unwrap()
	if len(causes) == 0 {
		return false
	}
	for _, c := range causes {
		if !alreadyReported(c) {
			return false
		}
	}
	return true
}
