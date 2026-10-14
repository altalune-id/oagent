package boot

import (
	"context"
	"fmt"
	"log/slog"

	natsgo "github.com/nats-io/nats.go"

	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/kernel"
	"altalune.id/oagent/internal/kernel/queue"
	natsqueue "altalune.id/oagent/internal/kernel/queue/nats"
)

//nolint:gochecknoglobals // test seam.
var buildQueueFn = buildQueue

func buildQueue(ctx context.Context, k kernel.Kernel, cfg config.Config, nc *natsgo.Conn, log *slog.Logger) (queue.Queue, error) {
	if !cfg.Queue.Enabled {
		return queue.Disabled(log), nil
	}
	switch cfg.Queue.Driver {
	case config.QueueDriverNATS:
		q, err := natsqueue.Open(ctx, nc, natsqueue.Options{Log: log, Tracer: k.Tracer, Meter: k.Meter, Unexpected: k.Reporter.Unexpected})
		if err != nil {
			return nil, err
		}
		return q, nil
	default:
		return nil, fmt.Errorf("boot: queue.driver %q has no driver", cfg.Queue.Driver)
	}
}
