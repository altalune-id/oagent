package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Worker names the consumer and the listener register under.
const (
	WorkerName   = "queue.consumer"
	ListenerName = "queue.listener"
)

// Delivery limits every driver applies to a job.
const (
	MaxAttempts       = 5
	HandlerTimeout    = 25 * time.Second
	DLQPublishTimeout = 4 * time.Second
)

// Handler binds one job to the domain code that runs it.
type Handler struct {
	Job            Job
	Handle         func(ctx context.Context, m Message) error
	OnDeadLetter   func(ctx context.Context, d DeadLetter) error
	SuppressReport bool
}

// Message is one delivery as a handler or listener sees it.
type Message struct {
	ID           uuid.UUID
	Name         string
	Version      int
	Data         []byte
	NumDelivered int
	CreatedAt    time.Time
}

// DeadLetter is a message the consumer moved to the dead-letter queue, with why.
type DeadLetter struct {
	Message
	Reason string
	Cause  string
}

// Provider is a domain that contributes queue handlers.
type Provider interface{ ConsumerHandlers() []Handler }

// Listener binds one broadcast to the code that refreshes this instance's state.
type Listener struct {
	Broadcast Broadcast
	Handle    func(ctx context.Context, m Message) error
}

// ListenerProvider is implemented by anything that owns per-instance state a broadcast refreshes.
type ListenerProvider interface{ Listeners() []Listener }

// Decode unmarshals m.Data into T.
func Decode[T any](m Message) (T, error) {
	var v T
	if err := json.Unmarshal(m.Data, &v); err != nil {
		var zero T
		return zero, fmt.Errorf("queue: decode %s v%d: %w", m.Name, m.Version, err)
	}
	return v, nil
}

// Permanent marks err as not worth retrying.
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return &PermanentError{Cause: err}
}
