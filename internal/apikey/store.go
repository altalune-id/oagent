package apikey

import (
	"context"
	"time"

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
	BySecretHash(ctx context.Context, hash [32]byte) (*APIKey, error)
}

// Writer writes APIKey aggregates.
type Writer interface {
	Save(ctx context.Context, k *APIKey) error
	TouchLastUsed(ctx context.Context, used map[uuid.UUID]time.Time) error
}
