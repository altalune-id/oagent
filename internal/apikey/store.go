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
	// ByHint returns orgID's live key whose secret ends with hint, or a *NotFoundError.
	ByHint(ctx context.Context, orgID uuid.UUID, hint string) (*APIKey, error)
	List(ctx context.Context, orgID uuid.UUID) ([]*APIKey, error)
	BySecretHash(ctx context.Context, hash [32]byte) (*APIKey, error)
}

// Writer writes APIKey aggregates.
type Writer interface {
	// Save upserts k; a new key whose hint a live key in its org already has is a *HintTakenError.
	Save(ctx context.Context, k *APIKey) error
	TouchLastUsed(ctx context.Context, used map[uuid.UUID]time.Time) error
	// RevokePersonalOf sets revoked_at to at on ownerID's live personal keys in orgID and reports how many; it joins the transaction on ctx.
	RevokePersonalOf(ctx context.Context, orgID, ownerID uuid.UUID, at time.Time) (int, error)
}
