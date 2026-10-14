package auth

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// ResetStore persists password resets. A missing row is a *ResetNotUsableError with ResetNotFound.
type ResetStore interface {
	Insert(ctx context.Context, r *Reset) error
	// DeleteUnused removes userID's resets that were never used.
	DeleteUnused(ctx context.Context, userID uuid.UUID) error
	// HasRecentUnused reports whether userID has an unused reset created after since.
	HasRecentUnused(ctx context.Context, userID uuid.UUID, since time.Time) (bool, error)
	ByTokenHash(ctx context.Context, hash string) (*Reset, error)
	ByPublicID(ctx context.Context, publicID string) (*Reset, error)
	// MarkUsed sets used_at on publicID's reset at now, only while it is unused and unexpired, and returns it.
	MarkUsed(ctx context.Context, publicID string, now time.Time) (*Reset, error)
}
