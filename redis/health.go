package redis

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"altalune.id/oagent/worker"
)

const probeEvery = 5 * time.Second

var _ worker.Worker = (*Health)(nil)

// Health pings every 5s; Ready is false while Redis does not answer within the timeout.
type Health struct {
	client  goredis.UniversalClient
	timeout time.Duration
	log     *slog.Logger
	ready   atomic.Bool
}

// NewHealth starts ready, since Open only returns a client that answered.
func NewHealth(client goredis.UniversalClient, timeout time.Duration, log *slog.Logger) *Health {
	h := &Health{client: client, timeout: timeout, log: log}
	h.ready.Store(true)
	return h
}

// Name implements worker.Worker.
func (h *Health) Name() string { return "redis-health" }

// Ready reports whether the last ping succeeded.
func (h *Health) Ready() bool { return h.ready.Load() }

// Run implements worker.Worker: it probes now, then every 5s, until ctx is done.
func (h *Health) Run(ctx context.Context) error {
	h.probe(ctx)
	ticker := time.NewTicker(probeEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			h.probe(ctx)
		}
	}
}

func (h *Health) probe(ctx context.Context) {
	pingCtx, cancel := context.WithTimeout(ctx, h.timeout)
	defer cancel()
	err := h.client.Ping(pingCtx).Err()
	if ctx.Err() != nil {
		return
	}
	if was := h.ready.Swap(err == nil); was == (err == nil) || h.log == nil {
		return
	}
	if err != nil {
		h.log.WarnContext(ctx, "redis: not ready", slog.String("err", err.Error()))
		return
	}
	h.log.InfoContext(ctx, "redis: ready again")
}
