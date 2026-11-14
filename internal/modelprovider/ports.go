package modelprovider

import (
	"context"

	"github.com/google/uuid"
)

// Usage reports how published agent versions depend on a provider.
type Usage interface {
	ProviderUse(ctx context.Context, providerID uuid.UUID) (ProviderUse, error)
}

// ProviderUse says who uses a provider: published versions (models, price needs, transcription models) and the embedding model.
type ProviderUse struct {
	InUse               bool
	Models              map[string]bool
	TranscriptionModels []string
	EmbeddingModel      string
}

// Refs turns org and project public ids into ids and back for the handler.
type Refs interface {
	OrgID(ctx context.Context, publicID string) (id uuid.UUID, ok bool, err error)
	OrgPublicID(ctx context.Context, id uuid.UUID) (string, error)
	ProjectID(ctx context.Context, publicID string) (id, orgID uuid.UUID, ok bool, err error)
	ProjectPublicID(ctx context.Context, id uuid.UUID) (string, error)
}
