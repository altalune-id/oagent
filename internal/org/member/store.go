package member

import (
	"context"

	"github.com/google/uuid"
)

// Store persists memberships.
type Store interface {
	Add(ctx context.Context, m *Membership) error
	// AddIfAbsent inserts m unless the user already belongs to the org, and reports whether it did.
	AddIfAbsent(ctx context.Context, m *Membership) (bool, error)
	Find(ctx context.Context, orgID, userID uuid.UUID) (*Membership, error)
	ListByUser(ctx context.Context, userID uuid.UUID) ([]*Membership, error)
	CountOwners(ctx context.Context, orgID uuid.UUID) (int, error)
	// LockOwners locks orgID's owner rows until the transaction on ctx ends and returns their user ids; it fails outside a transaction.
	LockOwners(ctx context.Context, orgID uuid.UUID) ([]uuid.UUID, error)
	// LockMember returns userID's membership in orgID, locked FOR SHARE until the transaction on ctx ends; it fails outside a transaction.
	LockMember(ctx context.Context, orgID, userID uuid.UUID) (*Membership, error)
	UpdateRole(ctx context.Context, orgID, userID uuid.UUID, role Role) error
	Delete(ctx context.Context, orgID, userID uuid.UUID) error
	// ListByOrg returns orgID's members with their email and name, owners first, then admins, then members, each by email.
	ListByOrg(ctx context.Context, orgID uuid.UUID) ([]Member, error)
}
