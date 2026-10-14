package queue

import (
	"context"
	"log/slog"

	"altalune.id/oagent/worker"
)

var _ Queue = disabled{}

type disabled struct{}

// Disabled returns a Queue whose Submit and Emit do nothing and that runs no consumer or listener.
func Disabled(log *slog.Logger) Queue {
	if log != nil {
		log.Info("queue: disabled — Submit is a no-op")
	}
	return disabled{}
}

func (disabled) Declare(jobs []Job, broadcasts []Broadcast) error {
	_, err := NewDeclared(jobs, broadcasts)
	return err
}

func (disabled) Submit(context.Context, Job, any) error { return nil }

func (disabled) Emit(context.Context, Broadcast, any) error { return nil }

func (disabled) Consumer(_ context.Context, hs []Handler) (worker.Worker, error) {
	if err := CheckHandlers(hs); err != nil {
		return nil, err
	}
	return nil, &DisabledError{}
}

func (disabled) Listener(_ context.Context, ls []Listener) (worker.Worker, error) {
	if err := CheckListeners(ls); err != nil {
		return nil, err
	}
	return nil, &DisabledError{}
}
