package channel

import (
	"context"
	"log/slog"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/run"
	"altalune.id/oagent/worker"
)

var _ worker.Worker = (*Replier)(nil)

const (
	replyJobName  = "channel-reply"
	replyEvery    = 15 * time.Second
	replyBatch    = 50
	replyGiveUp   = 10 * time.Minute
	replyFirstTry = 15 * time.Second
	replyMaxWait  = 2 * time.Minute
)

// Tenants enumerates the org scopes one reply sweep covers, binding each to the ctx it passes to fn.
type Tenants interface {
	Each(ctx context.Context, fn func(ctx context.Context, tenantID string) error) error
}

// Replier sends a settled channel run's reply through its connection, part by part, and retries it from its channel-reply loop.
type Replier struct {
	conns    Connections
	store    ReplyStore
	provider Provider
	log      *slog.Logger

	tenants  Tenants
	idlePoll time.Duration
	wake     chan struct{}

	mu      sync.Mutex
	retries []time.Time
	noted   int
}

// NewReplier builds a Replier; a nil log logs to slog.Default.
func NewReplier(conns Connections, store ReplyStore, provider Provider, log *slog.Logger) *Replier {
	if log == nil {
		log = slog.Default()
	}
	return &Replier{conns: conns, store: store, provider: provider, log: log, idlePoll: replyEvery, wake: make(chan struct{}, 1)}
}

// WithSweep lets Run sweep due replies of every tenant, at least every idlePoll; a non-positive idlePoll keeps 15 s.
func (r *Replier) WithSweep(tenants Tenants, idlePoll time.Duration) *Replier {
	r.tenants = tenants
	if idlePoll > 0 {
		r.idlePoll = idlePoll
	}
	return r
}

// Wake asks the loop to sweep now; it never blocks, and wakes sent during a sweep collapse into one.
func (r *Replier) Wake() {
	select {
	case r.wake <- struct{}{}:
	default:
	}
}

// Name implements worker.Worker.
func (r *Replier) Name() string { return replyJobName }

// Run implements worker.Worker, sweeping at start, on a wake, at the earliest retry it knows of, and on the idle poll until ctx is done.
func (r *Replier) Run(ctx context.Context) error {
	if r.tenants == nil {
		r.log.Info("channel: reply loop idle, no tenants configured")
		return nil
	}
	t := time.NewTimer(0)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-r.wake:
		case <-t.C:
		}
		if r.sweep(ctx) {
			t.Reset(replyEvery)
			continue
		}
		t.Reset(r.nextSweep(time.Now()))
	}
}

func (r *Replier) sweep(ctx context.Context) (full bool) {
	r.dropDueRetries(time.Now())
	err := r.tenants.Each(ctx, func(tctx context.Context, tenantID string) error {
		tctx, cancel := context.WithTimeout(tctx, replyEvery)
		defer cancel()
		ids, err := r.store.DueReplies(tctx, time.Now(), replyBatch)
		if err != nil {
			r.sweepFailed(ctx, tenantID, err)
			return ctx.Err()
		}
		full = full || len(ids) >= replyBatch
		for _, id := range ids {
			if err := r.send(tctx, id); err != nil {
				r.sweepFailed(ctx, tenantID, err)
			}
		}
		return ctx.Err()
	})
	if err != nil && ctx.Err() == nil {
		r.sweepFailed(ctx, "", err)
	}
	return full
}

func (r *Replier) sweepFailed(ctx context.Context, tenantID string, err error) {
	if ctx.Err() != nil {
		return
	}
	r.log.ErrorContext(ctx, "channel: reply sweep", "tenant", tenantID, "err", err.Error())
	r.noteRetry(time.Now().Add(replyEvery))
}

func (r *Replier) nextSweep(now time.Time) time.Duration {
	wait := r.idlePoll
	r.mu.Lock()
	for _, at := range r.retries {
		wait = min(wait, at.Sub(now))
	}
	r.mu.Unlock()
	return max(wait, replyEvery)
}

func (r *Replier) dropDueRetries(now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.retries = slices.DeleteFunc(r.retries, func(at time.Time) bool { return !at.After(now) })
}

func (r *Replier) noteRetry(at time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.retries = append(r.retries, at)
	r.noted++
}

func (r *Replier) notedCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.noted
}

// Send sends runID's due reply from the first part not yet sent, and wakes the loop for any retry it leaves.
func (r *Replier) Send(ctx context.Context, runID uuid.UUID) error {
	before := r.notedCount()
	err := r.send(ctx, runID)
	if err != nil {
		r.noteRetry(time.Now().Add(replyEvery))
	}
	if r.notedCount() != before {
		r.Wake()
	}
	return err
}

// SECURITY: logs carry the run public id only, never the address or text.
func (r *Replier) send(ctx context.Context, runID uuid.UUID) error {
	job, err := r.store.ReplyJob(ctx, runID)
	if err != nil {
		return err
	}
	now := time.Now()
	switch job.State {
	case run.ReplyDue:
	case "":
		r.noteRetry(now.Add(replyEvery))
		return nil
	default:
		return nil
	}
	if job.NextAt.After(now) {
		r.noteRetry(job.NextAt)
	}
	log := r.log.With("run", job.RunPublicID)
	if !job.EndedAt.IsZero() && now.Sub(job.EndedAt) >= replyGiveUp {
		log.ErrorContext(ctx, "channel: reply not delivered in time; giving up")
		return r.store.ReplyEnd(ctx, runID, run.ReplyFailed)
	}
	if job.ConnectionID == uuid.Nil {
		log.WarnContext(ctx, "channel: reply skipped; the thread is detached from its connection")
		return r.store.ReplyEnd(ctx, runID, run.ReplyFailed)
	}
	conn, secrets, err := r.conns.Open(ctx, job.ConnectionID)
	if IsNotFoundError(err) {
		log.WarnContext(ctx, "channel: reply skipped; the connection is disabled or gone")
		return r.store.ReplyEnd(ctx, runID, run.ReplyFailed)
	}
	if err != nil {
		return r.retry(ctx, log, job, err)
	}
	parts := Format(job.Text)
	if job.To == "" || len(parts) == 0 {
		log.WarnContext(ctx, "channel: reply skipped; no address or no text")
		return r.store.ReplyEnd(ctx, runID, run.ReplyFailed)
	}
	for i := job.PartsSent; i < len(parts); i++ {
		reply := Reply{To: job.To, Text: parts[i], IdempotencyKey: "run:" + job.RunPublicID + ":" + strconv.Itoa(i)}
		if job.Kind == string(KindGroup) && i == 0 {
			reply.ReplyTo = job.QuoteID
		}
		if _, err := r.provider.Send(ctx, *conn, secrets, reply); err != nil {
			if IsKeyRefusedError(err) {
				if err := r.conns.Record(ctx, conn.ID, ResultKeyRefused); err != nil {
					log.WarnContext(ctx, "channel: record result", "err", err.Error())
				}
			}
			return r.retry(ctx, log, job, err)
		}
		if err := r.store.ReplyProgress(ctx, runID, i+1); err != nil {
			return err
		}
	}
	log.DebugContext(ctx, "channel: reply sent", "parts", len(parts))
	return r.store.ReplyEnd(ctx, runID, run.ReplySent)
}

func (r *Replier) retry(ctx context.Context, log *slog.Logger, job *run.ReplyJob, cause error) error {
	now := time.Now()
	if now.Sub(job.EndedAt) >= replyGiveUp {
		log.ErrorContext(ctx, "channel: reply not delivered; giving up", "attempts", job.Attempts+1, "err", cause.Error())
		return r.store.ReplyEnd(ctx, job.RunID, run.ReplyFailed)
	}
	log.WarnContext(ctx, "channel: reply not delivered; retrying", "attempt", job.Attempts+1, "err", cause.Error())
	next := now.Add(backoff(job.Attempts))
	if err := r.store.ReplyRetry(ctx, job.RunID, next); err != nil {
		return err
	}
	db.AfterCommit(ctx, func() { r.noteRetry(next) })
	return nil
}

func backoff(attempts int) time.Duration {
	wait := replyFirstTry
	for range attempts {
		if wait >= replyMaxWait {
			break
		}
		wait *= 2
	}
	return min(wait, replyMaxWait)
}
