package agent

import (
	"context"

	"github.com/google/uuid"
)

// Store persists Agent aggregates.
type Store interface {
	Reader
	Writer
}

// Reader reads Agent aggregates; implementations may use a read replica.
type Reader interface {
	ByID(ctx context.Context, id uuid.UUID) (*Agent, error)
	Resolve(ctx context.Context, publicID string) (Ref, error)
	List(ctx context.Context, projectID uuid.UUID) ([]*Agent, error)
	// Versions returns up to limit of the agent's published versions, newest first.
	Versions(ctx context.Context, agentID uuid.UUID, limit int) ([]*Version, error)
	// LatestVersion returns the agent's newest published version, or nil before the first publish.
	LatestVersion(ctx context.Context, agentID uuid.UUID) (*Version, error)
	// VersionByID loads one version of the org on ctx, published or draft; a run resumes on the version it was pinned to.
	VersionByID(ctx context.Context, id uuid.UUID) (*Version, error)
	// ProviderUse reports how the latest published versions in the org on ctx use a model provider; drafts never count.
	ProviderUse(ctx context.Context, providerID uuid.UUID) (ProviderUsage, error)
	// MCPServerInUse reports whether a latest published version in the org on ctx grants tools of a server.
	MCPServerInUse(ctx context.Context, serverID uuid.UUID) (bool, error)
	// KnowledgeSourceInUse reports whether a latest published version in the org on ctx searches a knowledge source.
	KnowledgeSourceInUse(ctx context.Context, sourceID uuid.UUID) (bool, error)
	// KnowledgeUse names, per knowledge source of a project, the agents whose latest published version searches it.
	KnowledgeUse(ctx context.Context, projectID uuid.UUID) (map[uuid.UUID][]Named, error)
	// MemoryStoreInUse reports whether a latest published version in the org on ctx remembers people in a memory store.
	MemoryStoreInUse(ctx context.Context, storeID uuid.UUID) (bool, error)
	// AgentPublicIDs names agents of the org on ctx by public id in one read; an agent that is gone is left out.
	AgentPublicIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]string, error)
	// MemoryUse names, per memory store of a project, the agents whose latest published version remembers people in it.
	MemoryUse(ctx context.Context, projectID uuid.UUID) (map[uuid.UUID][]Named, error)
}

// Named is an agent's public id and name.
type Named struct {
	ID       uuid.UUID
	PublicID string
	Name     string
}

// ProviderUsage says whether a provider is used, per chat model whether a cost limit needs its price, and its transcription models.
type ProviderUsage struct {
	InUse               bool
	Models              map[string]bool
	TranscriptionModels []string
}

// Writer writes Agent aggregates.
type Writer interface {
	Save(ctx context.Context, a *Agent) error
	Delete(ctx context.Context, id uuid.UUID) error
	// SaveDraft writes only the agent's draft config and its edit time.
	SaveDraft(ctx context.Context, a *Agent) error
	// MarkPublished writes only the agent's published version and time.
	MarkPublished(ctx context.Context, a *Agent) error
	// LockForPublish loads the agent and locks its row until the unit of work on ctx ends.
	LockForPublish(ctx context.Context, id uuid.UUID) (*Agent, error)
	InsertVersion(ctx context.Context, v *Version) error
	// UpsertDraftVersion stores draft v unless the agent already has a draft with its hash, and returns the stored row.
	UpsertDraftVersion(ctx context.Context, v *Version) (*Version, error)
}
