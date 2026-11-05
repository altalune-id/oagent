package modelprovider

import (
	"context"

	"github.com/google/uuid"
)

// Usage reports how published agent versions depend on a provider.
type Usage interface {
	ProviderUse(ctx context.Context, providerID uuid.UUID) (ProviderUse, error)
}

// ProviderUse says whether any published version uses a provider and, per model id it uses, whether it needs that model's price.
type ProviderUse struct {
	InUse  bool
	Models map[string]bool
}

// Refs turns org and project public ids into ids and back for the handler.
type Refs interface {
	OrgID(ctx context.Context, publicID string) (id uuid.UUID, ok bool, err error)
	OrgPublicID(ctx context.Context, id uuid.UUID) (string, error)
	ProjectID(ctx context.Context, publicID string) (id, orgID uuid.UUID, ok bool, err error)
	ProjectPublicID(ctx context.Context, id uuid.UUID) (string, error)
}
