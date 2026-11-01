package agent

import (
	"context"

	"github.com/google/uuid"
)

// Store persists Agent aggregates.
type Store interface {
	Reader
	Writer
}

// Reader reads Agent aggregates; implementations may use a read replica.
type Reader interface {
	ByID(ctx context.Context, id uuid.UUID) (*Agent, error)
	Resolve(ctx context.Context, publicID string) (Ref, error)
	List(ctx context.Context, projectID uuid.UUID) ([]*Agent, error)
	// Versions returns the agent's versions, newest first.
	Versions(ctx context.Context, agentID uuid.UUID) ([]*Version, error)
	// LatestVersion returns the agent's newest version, or nil before the first publish.
	LatestVersion(ctx context.Context, agentID uuid.UUID) (*Version, error)
}

// Writer writes Agent aggregates.
type Writer interface {
	Save(ctx context.Context, a *Agent) error
	Delete(ctx context.Context, id uuid.UUID) error
	// LockForPublish loads the agent and locks its row until the unit of work on ctx ends.
	LockForPublish(ctx context.Context, id uuid.UUID) (*Agent, error)
	InsertVersion(ctx context.Context, v *Version) error
}
