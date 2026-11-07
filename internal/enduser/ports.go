package enduser

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// KeyHints names an API key by its public hint, for the label of an end user that is a key; an unknown key has an empty hint.
type KeyHints interface {
	KeyHint(ctx context.Context, keyID uuid.UUID) (string, error)
}

// Seen is when an end user last wrote and how many threads they own or wrote in.
type Seen struct {
	Last    time.Time
	Threads int64
}

// PersonThread is a thread an end user owns or wrote in.
type PersonThread struct {
	PublicID       string
	OrgID, AgentID uuid.UUID
	Kind           string
	LastActivity   time.Time
}

// Activity reads end users' conversations; the run store implements it.
type Activity interface {
	Seen(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]Seen, error)
	Threads(ctx context.Context, id uuid.UUID) ([]PersonThread, error)
}

// Refs turns project and agent ids into public ids and back for the end-user handler.
type Refs interface {
	ProjectID(ctx context.Context, publicID string) (id, orgID uuid.UUID, ok bool, err error)
	ProjectPublicID(ctx context.Context, id uuid.UUID) (string, error)
	ResourcePublicID(ctx context.Context, orgID, id uuid.UUID) (publicID string, ok bool, err error)
}
