package channel

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/run"
	"altalune.id/oagent/scheduler"
)

const (
	replyJobName  = "channel-reply"
	replyEvery    = 15 * time.Second
	replyBatch    = 50
	replyGiveUp   = 10 * time.Minute
	replyFirstTry = 15 * time.Second
	replyMaxWait  = 2 * time.Minute
)

// Replier sends a settled channel run's reply through its connection, part by part, and retries it from the channel-reply job.
type Replier struct {
	conns    Connections
	store    ReplyStore
	provider Provider
	log      *slog.Logger
}

// NewReplier builds a Replier; a nil log logs to slog.Default.
func NewReplier(conns Connections, store ReplyStore, provider Provider, log *slog.Logger) *Replier {
	if log == nil {
		log = slog.Default()
	}
	return &Replier{conns: conns, store: store, provider: provider, log: log}
}

// Send sends runID's due reply from the first part not yet sent. SECURITY: logs carry the run public id only, never the address or text.
func (r *Replier) Send(ctx context.Context, runID uuid.UUID) error {
	job, err := r.store.ReplyJob(ctx, runID)
	if err != nil || job.State != run.ReplyDue {
		return err
	}
	log := r.log.With("run", job.RunPublicID)
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
	return r.store.ReplyRetry(ctx, job.RunID, now.Add(backoff(job.Attempts)))
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

// SchedulerJobs returns the per-tenant job channel-reply, which sends due replies every 15 s.
func (r *Replier) SchedulerJobs() []scheduler.Job {
	return []scheduler.Job{{
		Name: replyJobName, Scope: scheduler.ScopeTenant,
		Schedule: scheduler.MustEveryInterval(replyEvery, replyEvery/5), Timeout: replyEvery,
		Run: func(ctx context.Context) error {
			ids, err := r.store.DueReplies(ctx, time.Now(), replyBatch)
			if err != nil {
				return err
			}
			var errs []error
			for _, id := range ids {
				errs = append(errs, r.Send(ctx, id))
			}
			return errors.Join(errs...)
		},
	}}
}
