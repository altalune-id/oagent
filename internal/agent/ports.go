package agent

import (
	"context"

	"altalune.id/oagent/internal/kernel/events"
)

// Publisher fans a domain event out to its subscribers; the webhook module implements it.
type Publisher interface {
	Enqueue(ctx context.Context, t events.Type, data any) error
}
