package boot

import (
	"context"
	"log/slog"

	"altalune.id/auth-server/config"
	"altalune.id/auth-server/internal/kernel"
	"altalune.id/auth-server/internal/kernel/queue"
)

// queueConfig maps the operator config onto the queue's options.
func queueConfig(c config.QueueConfig) queue.Options {
	return queue.Options{
		URL:            c.URL,
		Token:          c.Token,
		ConnectTimeout: c.ConnectTimeout,
	}
}

//nolint:gochecknoglobals // test seam.
var buildQueueFn = buildQueue

// buildQueue connects to NATS when enabled, or returns a Disabled no-op client.
func buildQueue(ctx context.Context, k kernel.Kernel, cfg config.Config, log *slog.Logger) (*queue.Client, error) {
	if !cfg.Queue.Enabled {
		return queue.Disabled(log), nil
	}
	o := queueConfig(cfg.Queue)
	o.Log, o.Tracer, o.Meter, o.Unexpected = log, k.Tracer, k.Meter, k.Reporter.Unexpected
	return queue.Connect(ctx, o)
}
