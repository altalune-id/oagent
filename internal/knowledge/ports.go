package knowledge

import (
	"context"
	"io"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/storage"
	"altalune.id/oagent/internal/knowledge/embed"
)

// UploadPurpose is the storage purpose of every knowledge file: private, under its project.
//
//nolint:gochecknoglobals // a read-only declaration, like a constant.
var UploadPurpose = storage.Purpose{
	Name: "knowledge", OwnerKind: storage.OwnerProjects, Visibility: storage.Private,
	Types: []string{TypePDF, TypeDOCX, TypeMarkdown, TypeText}, MaxBytes: MaxUploadBytes,
}

// Files is the file storage uploads go through; BeginUpload, MarkReady and MarkDeleting join the caller's unit of work.
type Files interface {
	Enabled() bool
	BeginUpload(ctx context.Context, in storage.BeginUploadInput) (*storage.File, storage.Upload, error)
	Verify(ctx context.Context, fileID uuid.UUID, p storage.Purpose, owner storage.Owner) (*storage.File, error)
	MarkReady(ctx context.Context, fileID uuid.UUID) error
	MarkDeleting(ctx context.Context, fileID uuid.UUID) error
	File(ctx context.Context, id uuid.UUID) (*storage.File, error)
	Open(ctx context.Context, key string, maxBytes int64) (io.ReadCloser, error)
}

// UsedBy names an agent whose published version searches a source.
type UsedBy struct {
	AgentID       uuid.UUID
	AgentPublicID string
	AgentName     string
}

// Bindings says which published agent versions search a project's sources.
type Bindings interface {
	SourceInUse(ctx context.Context, sourceID uuid.UUID) (bool, error)
	SourcesUsedBy(ctx context.Context, projectID uuid.UUID) (map[uuid.UUID][]UsedBy, error)
}

// Projects gives a project's public id, which names its storage folder.
type Projects interface {
	ProjectPublicID(ctx context.Context, id uuid.UUID) (string, error)
}

// EmbeddingModel is what an embeddings call needs from a project's model provider. SECURITY: APIKey is a secret; never log it.
type EmbeddingModel struct {
	OpenAICompatible bool
	BaseURL          string
	APIKey           string
	Embedding        bool
	InputPriceMicros *int64
}

// Models opens a provider's model for embeddings; an unknown provider or model fails with the provider's own code.
type Models interface {
	EmbeddingModel(ctx context.Context, providerID uuid.UUID, model string) (EmbeddingModel, error)
	LockProvider(ctx context.Context, providerID uuid.UUID) error
}

// Embedder turns texts into vectors with one model.
type Embedder interface {
	Probe(ctx context.Context) (int, error)
	Embed(ctx context.Context, texts []string) (embed.Result, error)
}
