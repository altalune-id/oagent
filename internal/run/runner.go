package run

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

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
	Claim(ctx context.Context, threadID uuid.UUID, token string, ttl time.Duration) (ClaimResult, error)
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
	slots  chan struct{}
	wg     sync.WaitGroup
	mu     sync.Mutex
	// turns is the context held leases run under; nil while Run is not taking work.
	turns context.Context
}

// ObservedExecutor runs a turn whose events a streaming client observes.
type ObservedExecutor interface {
	ExecuteObserved(ctx context.Context, threadID uuid.UUID, token string, obs Observer) (*Run, error)
}

// Slot is one turn of the runner's capacity, reserved before its lease is claimed.
type Slot struct {
	r    *Runner
	once *sync.Once
}

// Release gives the slot back when the handler bails out before RunHeld; it is safe to call more than once.
func (s Slot) Release() {
	if s.once != nil {
		s.once.Do(func() { <-s.r.slots })
	}
}

var errNotRunning = errors.New("run: the runner is not taking work")

// Reserve takes a turn slot without blocking; false means the runner is at its limit.
func (r *Runner) Reserve() (Slot, bool) {
	select {
	case r.slots <- struct{}{}:
		return Slot{r: r, once: new(sync.Once)}, true
	default:
		return Slot{}, false
	}
}

// RunHeld runs the lease token already holds on k's thread, on slot; sink sees the first run only and closes when it settles.
func (r *Runner) RunHeld(slot Slot, k Kick, token string, sink *Sink) error {
	exec, ok := r.exec.(ObservedExecutor)
	r.mu.Lock()
	turns := r.turns
	if turns != nil && ok {
		r.wg.Add(1)
	}
	r.mu.Unlock()
	if turns == nil || !ok {
		slot.Release()
		sink.Close()
		return errNotRunning
	}
	go func() {
		defer r.wg.Done()
		defer slot.Release()
		r.held(turns, k, token, exec, sink)
	}()
	return nil
}

// NewRunner builds the runner over exec and leases.
func NewRunner(exec Executor, leases Leases, opts RunnerOptions) *Runner {
	opts = opts.withDefaults()
	return &Runner{
		exec: exec, leases: leases, opts: opts, in: make(chan Kick, opts.Buffer), log: slog.Default(),
		slots: make(chan struct{}, opts.MaxConcurrent),
	}
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

// Run takes kicks and held leases until ctx ends, drains in-flight turns, then interrupts the rest; it returns once every turn has.
func (r *Runner) Run(ctx context.Context) error {
	turns, stop := context.WithCancelCause(context.WithoutCancel(ctx))
	defer stop(nil)
	r.mu.Lock()
	r.turns = turns
	r.mu.Unlock()
loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case k := <-r.in:
			slot, ok := r.Reserve()
			if !ok {
				r.log.DebugContext(ctx, "run: at the turn limit; the kick waits for the reconciler", "thread", k.ThreadID.String())
				continue
			}
			r.wg.Go(func() {
				defer slot.Release()
				r.lease(turns, k)
			})
		}
	}
	r.mu.Lock()
	r.turns = nil
	r.mu.Unlock()
	done := make(chan struct{})
	go func() {
		r.wg.Wait()
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

func (r *Runner) held(ctx context.Context, k Kick, token string, exec ObservedExecutor, sink *Sink) {
	first := true
	r.loop(tenant.Into(ctx, tenant.Context{OrgID: k.OrgID}), k, token, func(ctx context.Context) (*Run, error) {
		if !first {
			return r.exec.Execute(ctx, k.ThreadID, token)
		}
		first = false
		defer sink.Close()
		return exec.ExecuteObserved(ctx, k.ThreadID, token, sink.Observe)
	})
	sink.Close()
}

func (r *Runner) lease(ctx context.Context, k Kick) {
	ctx = tenant.Into(ctx, tenant.Context{OrgID: k.OrgID})
	token := uuid.NewString()
	claim, err := r.leases.Claim(ctx, k.ThreadID, token, r.opts.LeaseTTL)
	if err != nil {
		r.log.ErrorContext(ctx, "run: claim", "thread", k.ThreadID.String(), "err", err.Error())
		return
	}
	if claim != Claimed {
		return
	}
	r.loop(ctx, k, token, func(ctx context.Context) (*Run, error) { return r.exec.Execute(ctx, k.ThreadID, token) })
}

func (r *Runner) loop(ctx context.Context, k Kick, token string, execute func(context.Context) (*Run, error)) {
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
		done, err := r.turn(ctx, k.ThreadID, token, execute)
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

func (r *Runner) turn(
	ctx context.Context, threadID uuid.UUID, token string, execute func(context.Context) (*Run, error),
) (*Run, error) {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	ctx, stop := context.WithTimeout(ctx, r.opts.TurnTimeout)
	defer stop()
	var wg sync.WaitGroup
	wg.Go(func() { r.heartbeat(ctx, cancel, threadID, token) })
	done, err := execute(ctx)
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
