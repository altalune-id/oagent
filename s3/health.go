package s3

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"altalune.id/oagent/worker"
)

const probeEvery = 30 * time.Second

var _ worker.Worker = (*Health)(nil)

// Health checks the bucket every 30s; Ready is false while it does not answer within the timeout.
type Health struct {
	client  *Client
	timeout time.Duration
	log     *slog.Logger
	ready   atomic.Bool
}

// NewHealth starts ready, since Open only returns a client whose bucket answered.
func NewHealth(client *Client, timeout time.Duration, log *slog.Logger) *Health {
	h := &Health{client: client, timeout: timeout, log: log}
	h.ready.Store(true)
	return h
}

// Name implements worker.Worker.
func (h *Health) Name() string { return "s3-health" }

// Ready reports whether the last check succeeded.
func (h *Health) Ready() bool { return h.ready.Load() }

// Run implements worker.Worker: it checks now, then every 30s, until ctx is done.
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
	probeCtx, cancel := context.WithTimeout(ctx, h.timeout)
	defer cancel()
	err := h.client.headBucket(probeCtx)
	if ctx.Err() != nil {
		return
	}
	if was := h.ready.Swap(err == nil); was == (err == nil) || h.log == nil {
		return
	}
	if err != nil {
		h.log.WarnContext(ctx, "s3: not ready", slog.String("err", err.Error()))
		return
	}
	h.log.InfoContext(ctx, "s3: ready again")
}
