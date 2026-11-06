package run

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"altalune.id/oagent/internal/kernel/tenant"
)

// TurnsPerLease is how many turns a claim may run before it releases the thread for fairness.
const TurnsPerLease = 5

var (
	errShutdown  = errors.New("run: shutting down")
	errLeaseLost = errors.New("run: lease lost")
)

// Kick asks for a thread's pending messages to be run; it is a hint, Postgres is the truth.
type Kick struct {
	OrgID    uuid.UUID `json:"orgId"`
	ThreadID uuid.UUID `json:"threadId"`
}

// Executor runs one turn of a leased thread.
type Executor interface {
	Execute(ctx context.Context, threadID uuid.UUID, token string) (*Run, error)
}

// Leases claims, extends and releases thread leases.
type Leases interface {
	Claim(ctx context.Context, threadID uuid.UUID, token string, ttl time.Duration) (bool, error)
	Heartbeat(ctx context.Context, threadID uuid.UUID, token string, ttl time.Duration) (cancel bool, err error)
	Release(ctx context.Context, threadID uuid.UUID, token string) error
}

// RunnerOptions bound the turn runner; zero fields take the defaults.
type RunnerOptions struct {
	MaxConcurrent int
	Buffer        int
	LeaseTTL      time.Duration
	Heartbeat     time.Duration
	DrainTimeout  time.Duration
	TurnTimeout   time.Duration
}

func (o RunnerOptions) withDefaults() RunnerOptions {
	pick := func(v, d time.Duration) time.Duration {
		if v > 0 {
			return v
		}
		return d
	}
	if o.MaxConcurrent < 1 {
		o.MaxConcurrent = 8
	}
	if o.Buffer < 1 {
		o.Buffer = o.MaxConcurrent * 4
	}
	o.LeaseTTL = pick(o.LeaseTTL, time.Minute)
	o.Heartbeat = pick(o.Heartbeat, 20*time.Second)
	o.DrainTimeout = pick(o.DrainTimeout, 25*time.Second)
	o.TurnTimeout = pick(o.TurnTimeout, defaultTurnTimeout)
	return o
}

// Runner is the run-turns worker: it owns every turn goroutine of this instance.
type Runner struct {
	exec   Executor
	leases Leases
	opts   RunnerOptions
	in     chan Kick
	kicker Kicker
	log    *slog.Logger
}

// NewRunner builds the runner over exec and leases.
func NewRunner(exec Executor, leases Leases, opts RunnerOptions) *Runner {
	opts = opts.withDefaults()
	return &Runner{exec: exec, leases: leases, opts: opts, in: make(chan Kick, opts.Buffer), log: slog.Default()}
}

// WithKicker re-kicks threads the runner left with work: after five turns, and on shutdown.
func (r *Runner) WithKicker(k Kicker) *Runner {
	r.kicker = k
	return r
}

// WithLogger logs to l.
func (r *Runner) WithLogger(l *slog.Logger) *Runner {
	r.log = l
	return r
}

// Name implements worker.Worker.
func (r *Runner) Name() string { return "run-turns" }

// Offer hands k to the runner without blocking; false means the channel was full and the kick was dropped.
func (r *Runner) Offer(k Kick) bool {
	select {
	case r.in <- k:
		return true
	default:
		return false
	}
}

// Run takes kicks until ctx ends, then gives in-flight turns the drain timeout before it interrupts them; it returns once every turn has.
func (r *Runner) Run(ctx context.Context) error {
	turns, stop := context.WithCancelCause(context.WithoutCancel(ctx))
	defer stop(nil)
	var g errgroup.Group
	g.SetLimit(r.opts.MaxConcurrent)
loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case k := <-r.in:
			if !g.TryGo(func() error { r.lease(turns, k); return nil }) {
				r.log.DebugContext(ctx, "run: at the turn limit; the kick waits for the reconciler", "thread", k.ThreadID.String())
			}
		}
	}
	done := make(chan struct{})
	go func() {
		_ = g.Wait()
		close(done)
	}()
	drain := time.NewTimer(r.opts.DrainTimeout)
	defer drain.Stop()
	select {
	case <-done:
	case <-drain.C:
		stop(errShutdown)
		<-done
	}
	return nil
}

func (r *Runner) lease(ctx context.Context, k Kick) {
	ctx = tenant.Into(ctx, tenant.Context{OrgID: k.OrgID})
	token := uuid.NewString()
	ok, err := r.leases.Claim(ctx, k.ThreadID, token, r.opts.LeaseTTL)
	if err != nil {
		r.log.ErrorContext(ctx, "run: claim", "thread", k.ThreadID.String(), "err", err.Error())
		return
	}
	if !ok {
		return
	}
	rekick, keep := false, false
	defer func() {
		bg := context.WithoutCancel(ctx)
		if keep {
			return
		}
		if err := r.leases.Release(bg, k.ThreadID, token); err != nil && !IsLeaseLostError(err) {
			r.log.ErrorContext(bg, "run: release", "thread", k.ThreadID.String(), "err", err.Error())
		}
		if rekick && r.kicker != nil {
			if err := r.kicker.Kick(bg, k); err != nil {
				r.log.WarnContext(bg, "run: re-kick", "thread", k.ThreadID.String(), "err", err.Error())
			}
		}
	}()
	for range TurnsPerLease {
		if ctx.Err() != nil {
			rekick = true
			return
		}
		done, err := r.turn(ctx, k.ThreadID, token)
		switch {
		case IsLeaseLostError(err):
			return
		case IsSettleError(err):
			keep = true
			r.log.ErrorContext(ctx, "run: settle failed; the lease is left to expire", "thread", k.ThreadID.String())
			return
		case isRuntimeUnavailable(err):
			return
		case IsRateLimitedError(err):
			r.log.InfoContext(ctx, "run: project rate limited; the messages wait", "thread", k.ThreadID.String())
			return
		}
		if err != nil {
			r.log.ErrorContext(ctx, "run: turn", "thread", k.ThreadID.String(), "err", err.Error())
			return
		}
		if done == nil {
			return
		}
		if done.Status == StatusInterrupted && ctx.Err() == nil {
			return
		}
	}
	rekick = true
}

func (r *Runner) turn(ctx context.Context, threadID uuid.UUID, token string) (*Run, error) {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	ctx, stop := context.WithTimeout(ctx, r.opts.TurnTimeout)
	defer stop()
	var wg sync.WaitGroup
	wg.Go(func() { r.heartbeat(ctx, cancel, threadID, token) })
	done, err := r.exec.Execute(ctx, threadID, token)
	cancel(nil)
	wg.Wait()
	return done, err
}

func (r *Runner) heartbeat(ctx context.Context, cancel context.CancelCauseFunc, threadID uuid.UUID, token string) {
	tick := time.NewTicker(r.opts.Heartbeat)
	defer tick.Stop()
	confirmed := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			stopping, err := r.leases.Heartbeat(ctx, threadID, token, r.opts.LeaseTTL)
			switch {
			case IsLeaseLostError(err):
				cancel(errLeaseLost)
				return
			case err != nil:
				if ctx.Err() != nil {
					return
				}
				r.log.WarnContext(ctx, "run: heartbeat", "thread", threadID.String(), "err", err.Error())
				if time.Since(confirmed) >= r.opts.LeaseTTL-r.opts.Heartbeat {
					cancel(errLeaseLost)
					return
				}
			case stopping:
				cancel(ErrCancelRequested)
				return
			default:
				confirmed = time.Now()
			}
		}
	}
}

func isRuntimeUnavailable(err error) bool {
	_, ok := errors.AsType[*RuntimeUnavailableError](err)
	return ok
}
