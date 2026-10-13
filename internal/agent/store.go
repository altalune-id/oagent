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
	ByPublicID(ctx context.Context, publicID string) (*Agent, error)
	List(ctx context.Context, projectID uuid.UUID) ([]*Agent, error)
}

// Writer writes Agent aggregates.
type Writer interface {
	Save(ctx context.Context, a *Agent) error
	Delete(ctx context.Context, id uuid.UUID) error
}
