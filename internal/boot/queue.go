package boot

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/nats-io/nats.go"

	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/kernel"
	"altalune.id/oagent/internal/kernel/queue"
)

//nolint:gochecknoglobals // test seam.
var buildQueueFn = buildQueue

func buildQueue(ctx context.Context, k kernel.Kernel, cfg config.Config, nc *nats.Conn, log *slog.Logger) (*queue.Client, error) {
	if !cfg.Queue.Enabled {
		return queue.Disabled(log), nil
	}
	switch cfg.Queue.Driver {
	case config.QueueDriverNATS:
		return queue.Open(ctx, nc, queue.Options{Log: log, Tracer: k.Tracer, Meter: k.Meter, Unexpected: k.Reporter.Unexpected})
	default:
		return nil, fmt.Errorf("boot: queue.driver %q has no driver", cfg.Queue.Driver)
	}
}
