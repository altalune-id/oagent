package mcpserver

import (
	"context"

	"github.com/google/uuid"
)

// Store persists Server aggregates.
type Store interface {
	Reader
	Writer
}

// Reader reads Server aggregates; implementations may use a read replica.
type Reader interface {
	ByID(ctx context.Context, id uuid.UUID) (*Server, error)
	Resolve(ctx context.Context, publicID string) (Ref, error)
	List(ctx context.Context, projectID uuid.UUID) ([]*Server, error)
}

// Writer writes Server aggregates; the lock methods need a unit of work on ctx and hold their lock until it ends.
type Writer interface {
	Save(ctx context.Context, s *Server) error
	Delete(ctx context.Context, id uuid.UUID) error
	LockForUpdate(ctx context.Context, id uuid.UUID) (*Server, error)
	LockShared(ctx context.Context, ids []uuid.UUID) error
}
