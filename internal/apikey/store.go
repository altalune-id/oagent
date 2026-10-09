package apikey

import (
	"context"

	"github.com/google/uuid"
)

// Store persists APIKey aggregates.
type Store interface {
	Reader
	Writer
}

// Reader reads APIKey aggregates; implementations may use a read replica.
type Reader interface {
	ByID(ctx context.Context, id uuid.UUID) (*APIKey, error)
	List(ctx context.Context, orgID uuid.UUID) ([]*APIKey, error)
}

// Writer writes APIKey aggregates.
type Writer interface {
	Save(ctx context.Context, k *APIKey) error
}
