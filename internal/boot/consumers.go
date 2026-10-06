package boot

import (
	"context"

	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/kernel/queue"
)

type (
	newConsumerFunc func(context.Context, *queue.Client, []queue.Handler) (*queue.Consumer, error)
	newListenerFunc func(*queue.Client, []queue.Listener, uint64) (*queue.Listen, error)
	startSeqFunc    func(context.Context, *queue.Client) (uint64, error)
)

//nolint:gochecknoglobals // test seams.
var (
	newConsumer newConsumerFunc = queue.NewConsumer
	newListener newListenerFunc = queue.NewListener
	startSeq    startSeqFunc    = func(ctx context.Context, c *queue.Client) (uint64, error) {
		return c.BroadcastStartSeq(ctx)
	}
)

// consumerOff returns the reason the queue consumer is disabled, or "" when it should run.
func consumerOff(o *options, cfg config.Config) string {
	switch {
	case !o.consumer:
		return "WithConsumer(false)"
	case o.schedulerOnly:
		return "WithSchedulerOnly"
	case !cfg.Queue.Enabled:
		return "queue.enabled=false"
	}
	return ""
}

// queueHandlers returns the domain queue handlers; empty until domains contribute them.
func queueHandlers(_ *Services) []queue.Handler { return nil }

// queueListeners returns the per-instance broadcast listeners; empty until domains contribute them.
func queueListeners(_ *Services) []queue.Listener { return nil }
