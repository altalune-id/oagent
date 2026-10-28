// Package usage batches last-used writes on a ticker, so authenticating a credential never waits on one.
package usage

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/tenant"
)

const drainTimeout = 5 * time.Second

// Toucher moves the last-used time of each record forward, under the tenant scope on ctx.
type Toucher interface {
	TouchLastUsed(ctx context.Context, used map[uuid.UUID]time.Time) error
}

// Worker records last-used times in memory and flushes them per tenant every interval.
type Worker struct {
	name     string
	store    Toucher
	interval time.Duration
	log      *slog.Logger

	mu      sync.Mutex
	pending map[tenant.Context]map[uuid.UUID]time.Time
}

// New returns a Worker called name that flushes to store every interval.
func New(name string, store Toucher, interval time.Duration, log *slog.Logger) *Worker {
	if log == nil {
		log = slog.Default()
	}
	return &Worker{
		name: name, store: store, interval: interval, log: log.With("module", name),
		pending: make(map[tenant.Context]map[uuid.UUID]time.Time),
	}
}

// Record queues the last-used time of the record id in scope for the next flush, keeping the latest per record.
func (w *Worker) Record(scope tenant.Context, id uuid.UUID, at time.Time) {
	w.mu.Lock()
	defer w.mu.Unlock()
	batch := w.pending[scope]
	if batch == nil {
		batch = make(map[uuid.UUID]time.Time)
		w.pending[scope] = batch
	}
	if cur, ok := batch[id]; !ok || at.After(cur) {
		batch[id] = at
	}
}

// Name implements worker.Worker.
func (w *Worker) Name() string { return w.name }

// Run implements worker.Worker, flushing on a ticker until ctx is done and once more on the way out.
func (w *Worker) Run(ctx context.Context) error {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			out, cancel := context.WithTimeout(context.WithoutCancel(ctx), drainTimeout)
			w.flush(out)
			cancel()
			return ctx.Err()
		case <-ticker.C:
			w.flush(ctx)
		}
	}
}

// SECURITY: each tenant's batch is written under its own scope, so row-level security holds for every record.
func (w *Worker) flush(ctx context.Context) {
	w.mu.Lock()
	if len(w.pending) == 0 {
		w.mu.Unlock()
		return
	}
	pending := w.pending
	w.pending = make(map[tenant.Context]map[uuid.UUID]time.Time)
	w.mu.Unlock()

	for scope, batch := range pending {
		if err := w.store.TouchLastUsed(tenant.Into(ctx, scope), batch); err != nil {
			w.log.ErrorContext(ctx, w.name+" flush failed", slog.Int("records", len(batch)), slog.Any("err", err))
		}
	}
}
