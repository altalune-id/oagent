package run

import (
	"context"
	"fmt"

	"altalune.id/oagent/internal/kernel/queue"
)

// TurnJob is the queued job that kicks a thread on the instance that consumes it.
//
//nolint:gochecknoglobals // the job's fixed name and version.
var TurnJob = queue.Job{Name: "run.turn", Version: 1}

// Kicker asks for a thread to be run soon.
type Kicker interface {
	Kick(ctx context.Context, k Kick) error
}

// NewKicker submits run.turn jobs to q when the queue is on, else hands kicks straight to the local runner.
func NewKicker(q queue.Queue, queueOn bool, r *Runner) Kicker {
	if queueOn {
		return queueKicker{q: q}
	}
	return localKicker{r: r}
}

type queueKicker struct{ q queue.Queue }

func (k queueKicker) Kick(ctx context.Context, kick Kick) error {
	if err := k.q.Submit(ctx, TurnJob, kick); err != nil {
		return fmt.Errorf("run: submit kick: %w", err)
	}
	return nil
}

type localKicker struct{ r *Runner }

func (k localKicker) Kick(_ context.Context, kick Kick) error {
	k.r.Offer(kick)
	return nil
}

// Handler is the run.turn consumer: it hands the kick to the runner and acks at once, full or not.
func (r *Runner) Handler() queue.Handler {
	return queue.Handler{
		Job: TurnJob,
		Handle: func(_ context.Context, m queue.Message) error {
			k, err := queue.Decode[Kick](m)
			if err != nil {
				return queue.Permanent(fmt.Errorf("run: decode kick: %w", err))
			}
			r.Offer(k)
			return nil
		},
	}
}
