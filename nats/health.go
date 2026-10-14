package nats

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	natsgo "github.com/nats-io/nats.go"

	"altalune.id/oagent/worker"
)

const probeEvery = 5 * time.Second

var _ worker.Worker = (*Health)(nil)

// Health samples the connection every 5s; Ready is false while it is not connected.
type Health struct {
	nc    *natsgo.Conn
	log   *slog.Logger
	ready atomic.Bool
}

// NewHealth starts ready, since Open only returns a connected nc.
func NewHealth(nc *natsgo.Conn, log *slog.Logger) *Health {
	h := &Health{nc: nc, log: log}
	h.ready.Store(true)
	return h
}

// Name implements worker.Worker.
func (h *Health) Name() string { return "nats-health" }

// Ready reports whether the last sample found the connection connected.
func (h *Health) Ready() bool { return h.ready.Load() }

// Run implements worker.Worker: it samples now, then every 5s, until ctx is done.
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
	ok := h.nc.IsConnected()
	if was := h.ready.Swap(ok); was == ok || h.log == nil {
		return
	}
	if !ok {
		h.log.WarnContext(ctx, "nats: not ready", slog.String("status", h.nc.Status().String()))
		return
	}
	h.log.InfoContext(ctx, "nats: ready again")
}
