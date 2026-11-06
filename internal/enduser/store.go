package enduser

import (
	"context"

	"github.com/google/uuid"
)

// Store persists EndUser aggregates and their identities.
type Store interface {
	Reader
	Writer
}

// Reader reads end users; implementations may use a read replica.
type Reader interface {
	ByID(ctx context.Context, id uuid.UUID) (*EndUser, error)
	Owners(ctx context.Context, projectID uuid.UUID, ids []Identity) (map[Identity]uuid.UUID, error)
}

// Writer writes end users and their identities.
type Writer interface {
	Save(ctx context.Context, e *EndUser) error
	AddIdentity(ctx context.Context, e *EndUser, id Identity) (added bool, err error)
}
