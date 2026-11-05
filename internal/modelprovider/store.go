package modelprovider

import (
	"context"

	"github.com/google/uuid"
)

// Store persists Provider aggregates.
type Store interface {
	Reader
	Writer
}

// Reader reads Provider aggregates; implementations may use a read replica.
type Reader interface {
	ByID(ctx context.Context, id uuid.UUID) (*Provider, error)
	Resolve(ctx context.Context, publicID string) (Ref, error)
	List(ctx context.Context, projectID uuid.UUID) ([]*Provider, error)
}

// Writer writes Provider aggregates; the lock methods need a unit of work on ctx and hold their lock until it ends.
type Writer interface {
	Save(ctx context.Context, p *Provider) error
	Delete(ctx context.Context, id uuid.UUID) error
	LockForUpdate(ctx context.Context, id uuid.UUID) (*Provider, error)
	LockShared(ctx context.Context, ids []uuid.UUID) error
}
