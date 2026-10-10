package member

import (
	"context"

	"github.com/google/uuid"
)

// Store persists memberships.
type Store interface {
	Add(ctx context.Context, m *Membership) error
	Find(ctx context.Context, orgID, userID uuid.UUID) (*Membership, error)
	ListByUser(ctx context.Context, userID uuid.UUID) ([]*Membership, error)
	CountOwners(ctx context.Context, orgID uuid.UUID) (int, error)
}
