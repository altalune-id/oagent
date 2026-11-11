package outbox

import (
	"context"
	"log/slog"
	"slices"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"altalune.id/oagent/worker"
)

var _ worker.Worker = (*Worker)(nil)

// WorkerName is the name the dispatch worker registers under on a Supervisor.
const WorkerName = "outbox.dispatch"

const (
	defaultTick        = 5 * time.Second
	defaultBatch       = 100
	defaultConcurrency = 8
	defaultSettleGrace = 5 * time.Second
)

// Deliverer hands one claimed entry to the outbound transport; the webhook module ships the implementation.
type Deliverer interface {
	Deliver(ctx context.Context, e Entry) error
}

// Tenants enumerates the org scopes one sweep covers, binding each to the ctx it passes to fn.
type Tenants interface {
	Each(ctx context.Context, fn func(ctx context.Context, tenantID string) error) error
}

// WorkerOpts tunes one dispatch loop, with every non-positive field taking the package default.
type WorkerOpts struct {
	// Tick is the shortest wait between two sweeps that no wake asked for.
	Tick time.Duration
	// IdlePoll is the longest wait between two sweeps when no wake or retry is due.
	IdlePoll time.Duration
	// Batch caps how many entries one tenant yields per sweep.
	Batch int
	// Concurrency caps deliveries in flight within one tenant's batch.
	Concurrency int
	// SettleGrace bounds the Succeed or Fail that records a delivery outcome during shutdown.
	SettleGrace time.Duration
}

// Worker claims due entries on a wake, a retry or the idle poll, and hands each to a Deliverer.
type Worker struct {
	store   Store
	deliver Deliverer
	tenants Tenants
	log     *slog.Logger
	opts    WorkerOpts
	wake    chan struct{}

	mu      sync.Mutex
	retries []time.Time
}

// NewWorker returns the dispatch worker, whose Run is a no-op when Store, Deliverer or Tenants is nil.
func NewWorker(s Store, d Deliverer, tenants Tenants, log *slog.Logger, opts WorkerOpts) *Worker {
	if log == nil {
		log = slog.Default()
	}
	if opts.Tick <= 0 {
		opts.Tick = defaultTick
	}
	if opts.IdlePoll <= 0 {
		opts.IdlePoll = opts.Tick
	}
	if opts.Batch <= 0 {
		opts.Batch = defaultBatch
	}
	if opts.Concurrency <= 0 {
		opts.Concurrency = defaultConcurrency
	}
	if opts.SettleGrace <= 0 {
		opts.SettleGrace = defaultSettleGrace
	}
	return &Worker{
		store:   s,
		deliver: d,
		tenants: tenants,
		log:     log.With(slog.String("component", WorkerName)),
		opts:    opts,
		wake:    make(chan struct{}, 1),
	}
}

// Wake asks the worker to sweep now; it never blocks, and wakes sent during a sweep collapse into one.
func (w *Worker) Wake() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

// Name implements worker.Worker.
func (w *Worker) Name() string { return WorkerName }

// Run implements worker.Worker, sweeping at start, on a wake, at the earliest retry it scheduled, and on the idle poll until ctx is done.
func (w *Worker) Run(ctx context.Context) error {
	if w.store == nil || w.deliver == nil || w.tenants == nil {
		w.log.Info("outbox: dispatch idle, no deliverer configured")
		return nil
	}
	t := time.NewTimer(0)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-w.wake:
		case <-t.C:
		}
		if w.sweep(ctx) {
			t.Reset(w.opts.Tick)
			continue
		}
		t.Reset(w.nextSweep(time.Now()))
	}
}

func (w *Worker) sweep(ctx context.Context) (full bool) {
	w.dropDueRetries(time.Now())
	err := w.tenants.Each(ctx, func(tctx context.Context, tenantID string) error {
		if w.drain(tctx, tenantID) {
			full = true
		}
		return ctx.Err()
	})
	if err == nil || ctx.Err() != nil {
		return full
	}
	w.log.ErrorContext(ctx, "outbox: enumerate tenants", slog.String("err", err.Error()))
	w.retryAt(time.Now().Add(w.opts.Tick))
	return full
}

func (w *Worker) nextSweep(now time.Time) time.Duration {
	wait := w.opts.IdlePoll
	w.mu.Lock()
	for _, at := range w.retries {
		wait = min(wait, at.Sub(now))
	}
	w.mu.Unlock()
	return max(wait, w.opts.Tick)
}

func (w *Worker) dropDueRetries(now time.Time) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.retries = slices.DeleteFunc(w.retries, func(at time.Time) bool { return !at.After(now) })
}

func (w *Worker) retryAt(at time.Time) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.retries = append(w.retries, at)
}

func (w *Worker) drain(ctx context.Context, tenantID string) (full bool) {
	claimed, err := w.store.ClaimDue(ctx, time.Now().UTC(), w.opts.Batch)
	if err != nil {
		if ctx.Err() != nil {
			return false
		}
		w.log.ErrorContext(ctx, "outbox: claim due",
			slog.String("tenant", tenantID), slog.String("err", err.Error()))
		w.retryAt(time.Now().Add(w.opts.Tick))
		return false
	}
	if len(claimed) == 0 {
		return false
	}
	g := new(errgroup.Group)
	g.SetLimit(w.opts.Concurrency)
	for _, e := range claimed {
		if ctx.Err() != nil {
			break
		}
		g.Go(func() error {
			// NOTE: a claim not yet started at cancel skips Deliver and Fail; its lease releases it.
			if ctx.Err() == nil {
				w.deliverOne(ctx, e)
			}
			return nil
		})
	}
	_ = g.Wait()
	return len(claimed) >= w.opts.Batch
}

func (w *Worker) deliverOne(ctx context.Context, e Entry) {
	err := w.deliver.Deliver(ctx, e)

	// NOTE: settling is detached from ctx so a cancel mid-delivery still records the outcome.
	settleCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), w.opts.SettleGrace)
	defer cancel()

	if err == nil {
		if sErr := w.store.Succeed(settleCtx, e, time.Now().UTC()); sErr != nil {
			w.log.ErrorContext(settleCtx, "outbox: settle delivered",
				slog.String("entry", e.ID.String()), slog.String("err", sErr.Error()))
			w.retryAt(time.Now().Add(ClaimLease))
		}
		return
	}

	retryAt := time.Now().UTC().Add(Backoff(e.Attempt + 1))
	if fErr := w.store.Fail(settleCtx, e, retryAt, err.Error()); fErr != nil {
		w.log.ErrorContext(settleCtx, "outbox: settle failed delivery",
			slog.String("entry", e.ID.String()), slog.String("err", fErr.Error()))
		w.retryAt(time.Now().Add(ClaimLease))
		return
	}
	if e.Attempt < MaxAttempts {
		w.retryAt(retryAt)
	}
}
