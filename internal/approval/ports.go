package approval

import (
	"context"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/events"
)

// Publisher fans an approval event out to its subscribers; the webhook module implements it.
type Publisher interface {
	Enqueue(ctx context.Context, t events.Type, data any) error
}

// Refs names the project, agent, thread and run of an approval by public id, for its events.
type Refs interface {
	ProjectPublicID(ctx context.Context, id uuid.UUID) (string, error)
	ResourcePublicID(ctx context.Context, orgID, id uuid.UUID) (publicID string, ok bool, err error)
	ThreadPublicID(ctx context.Context, id uuid.UUID) (string, error)
	RunPublicID(ctx context.Context, id uuid.UUID) (string, error)
}

// HandlerRefs adds the lookups the approval handler needs to the event refs: project and agent public ids in, a member's public id out.
type HandlerRefs interface {
	Refs
	ProjectID(ctx context.Context, publicID string) (id, orgID uuid.UUID, ok bool, err error)
	ResourceID(ctx context.Context, orgID uuid.UUID, publicID string) (uuid.UUID, bool, error)
	UserPublicID(ctx context.Context, id uuid.UUID) (string, error)
}

// Names labels the people around an approval with the end-user label rule; no identity value is returned.
type Names interface {
	EndUser(ctx context.Context, id uuid.UUID) (publicID, label string, err error)
	Member(ctx context.Context, userID uuid.UUID) (string, error)
	KeyHint(ctx context.Context, keyID uuid.UUID) (string, error)
}

// Kicker wakes a thread's runner after a decision; the run kicker implements it.
type Kicker interface {
	Kick(ctx context.Context, orgID, threadID uuid.UUID) error
}
