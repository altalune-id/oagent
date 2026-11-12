package channel

import (
	"context"
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/events"
	"altalune.id/oagent/internal/run"
)

// Publisher fans a connection event out to its subscribers; the webhook module implements it.
type Publisher interface {
	Enqueue(ctx context.Context, t events.Type, data any) error
}

// Refs turns org, project and agent public ids into ids and back, for events and the control plane.
type Refs interface {
	OrgPublicID(ctx context.Context, id uuid.UUID) (string, error)
	ProjectID(ctx context.Context, publicID string) (id, orgID uuid.UUID, ok bool, err error)
	ProjectPublicID(ctx context.Context, id uuid.UUID) (string, error)
	ResourceID(ctx context.Context, orgID uuid.UUID, publicID string) (uuid.UUID, bool, error)
	ResourcePublicID(ctx context.Context, orgID, id uuid.UUID) (publicID string, ok bool, err error)
}

// ReplyStore reads and advances channel replies; the run store implements it.
type ReplyStore interface {
	ReplyJob(ctx context.Context, runID uuid.UUID) (*run.ReplyJob, error)
	DueReplies(ctx context.Context, now time.Time, limit int) ([]uuid.UUID, error)
	ReplyProgress(ctx context.Context, runID uuid.UUID, kind string, partsSent int) error
	ReplyRetry(ctx context.Context, runID uuid.UUID, kind string, next time.Time) error
	ReplyEnd(ctx context.Context, runID uuid.UUID, kind, state string) error
}

// Connections opens an active connection with its secrets and records a verified outcome; *Service implements it.
type Connections interface {
	Open(ctx context.Context, id uuid.UUID) (*Connection, Secrets, error)
	Record(ctx context.Context, id uuid.UUID, result string) error
}
