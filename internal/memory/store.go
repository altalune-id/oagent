package memory

import (
	"context"

	"github.com/google/uuid"
)

// Store persists memory stores.
type Store interface {
	InsertStore(ctx context.Context, s *MemoryStore) error
	StoreByID(ctx context.Context, id uuid.UUID) (*MemoryStore, error)
	ListStores(ctx context.Context, projectID uuid.UUID) ([]*MemoryStore, error)
	ResolveStore(ctx context.Context, publicID string) (Ref, error)
	DeleteStore(ctx context.Context, id uuid.UUID) error
	LockStoreShared(ctx context.Context, id uuid.UUID) error
	LockStore(ctx context.Context, id uuid.UUID) (*MemoryStore, error)
}
