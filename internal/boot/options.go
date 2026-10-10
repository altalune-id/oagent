package boot

import "log/slog"

// Option tunes what Boot wires and starts.
type Option func(*options)

type options struct {
	scheduler     bool
	schedulerOnly bool
	consumer      bool
	consumerOnly  bool
	installURL    bool
	logger        *slog.Logger
}

func newOptions() *options { return &options{scheduler: true, consumer: true, installURL: true} }

// WithScheduler enables or disables the periodic-job runner, overriding scheduler.enabled.
func WithScheduler(on bool) Option { return func(o *options) { o.scheduler = on } }

// WithSchedulerOnly runs the scheduler and a health-only listener, with no web handler.
func WithSchedulerOnly(on bool) Option { return func(o *options) { o.schedulerOnly = on } }

// WithConsumer enables or disables the queue job consumer on this instance.
func WithConsumer(on bool) Option { return func(o *options) { o.consumer = on } }

// WithConsumerOnly runs the queue consumer and a health-only listener, with no scheduler or web handler.
func WithConsumerOnly(on bool) Option { return func(o *options) { o.consumerOnly = on } }

// WithInstallURL sets whether Boot logs the one-time install URL; turn it off for a process that never serves /install.
func WithInstallURL(on bool) Option { return func(o *options) { o.installURL = on } }

// WithLogger replaces the logger Boot uses, so an embedder or test can observe boot output.
func WithLogger(l *slog.Logger) Option { return func(o *options) { o.logger = l } }
