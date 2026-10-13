package project

import (
	"context"

	"github.com/google/uuid"
)

// Store persists Project aggregates.
type Store interface {
	Reader
	Writer
}

// Reader reads Project aggregates; implementations may use a read replica.
type Reader interface {
	ByID(ctx context.Context, id uuid.UUID) (*Project, error)
	ByPublicID(ctx context.Context, publicID string) (*Project, error)
	BySlug(ctx context.Context, orgID uuid.UUID, slug string) (*Project, error)
	List(ctx context.Context, orgID uuid.UUID) ([]*Project, error)
}

// Writer writes Project aggregates.
type Writer interface {
	Save(ctx context.Context, p *Project) error
}
