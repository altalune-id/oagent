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
	// Versions returns up to limit of the agent's versions, newest first.
	Versions(ctx context.Context, agentID uuid.UUID, limit int) ([]*Version, error)
	// LatestVersion returns the agent's newest version, or nil before the first publish.
	LatestVersion(ctx context.Context, agentID uuid.UUID) (*Version, error)
	// ProviderUse reports how the latest published versions in the org on ctx use a model provider; drafts never count.
	ProviderUse(ctx context.Context, providerID uuid.UUID) (ProviderUsage, error)
	// MCPServerInUse reports whether a latest published version in the org on ctx grants tools of a server.
	MCPServerInUse(ctx context.Context, serverID uuid.UUID) (bool, error)
}

// ProviderUsage says whether a provider is used and, per model id used, whether a cost limit needs its price.
type ProviderUsage struct {
	InUse  bool
	Models map[string]bool
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
}
