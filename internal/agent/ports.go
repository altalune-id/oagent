package agent

import (
	"context"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/events"
)

// Publisher fans a domain event out to its subscribers; the webhook module implements it.
type Publisher interface {
	Enqueue(ctx context.Context, t events.Type, data any) error
}

// Refs turns org, project, provider, MCP server and knowledge source public ids into ids and back; ok is false for an unknown id.
type Refs interface {
	OrgID(ctx context.Context, publicID string) (id uuid.UUID, ok bool, err error)
	OrgPublicID(ctx context.Context, id uuid.UUID) (string, error)
	ProjectID(ctx context.Context, publicID string) (id, orgID uuid.UUID, ok bool, err error)
	ProjectPublicID(ctx context.Context, id uuid.UUID) (string, error)
	UserPublicID(ctx context.Context, id uuid.UUID) (string, error)
	ProviderID(ctx context.Context, publicID string) (id, projectID uuid.UUID, ok bool, err error)
	ProviderPublicID(ctx context.Context, id uuid.UUID) (publicID string, ok bool, err error)
	MCPServerID(ctx context.Context, publicID string) (id, projectID uuid.UUID, ok bool, err error)
	MCPServerPublicID(ctx context.Context, id uuid.UUID) (publicID string, ok bool, err error)
	KnowledgeSourceID(ctx context.Context, publicID string) (id, projectID uuid.UUID, ok bool, err error)
	KnowledgeSourcePublicID(ctx context.Context, id uuid.UUID) (publicID string, ok bool, err error)
	MemoryStoreID(ctx context.Context, publicID string) (id, projectID uuid.UUID, ok bool, err error)
	MemoryStorePublicID(ctx context.Context, id uuid.UUID) (publicID string, ok bool, err error)
}

// ModelRef is one model a provider lists; Priced is true when it has prices.
type ModelRef struct {
	ID            string
	Priced        bool
	Transcription bool
}

// Providers lists a project's provider models; an unknown or foreign provider answers *InvalidGrantError (provider).
type Providers interface {
	Models(ctx context.Context, projectID, providerID uuid.UUID) ([]ModelRef, error)
}

// MCPServers names a project's MCP server; an unknown or foreign server answers *InvalidGrantError (server).
type MCPServers interface {
	Exists(ctx context.Context, projectID, serverID uuid.UUID) (name string, err error)
}

// Knowledge checks a config's knowledge sources; an unknown, foreign or gone one answers *InvalidGrantError (knowledge_source).
type Knowledge interface {
	SourceExists(ctx context.Context, projectID, sourceID uuid.UUID) error
	LockSourcesShared(ctx context.Context, ids []uuid.UUID) error
}

// Memory checks a config's memory store; a bad one answers *InvalidGrantError (memory_store or memory_store_disabled).
type Memory interface {
	StoreUsable(ctx context.Context, projectID, storeID uuid.UUID) error
	LockStoreShared(ctx context.Context, id uuid.UUID) error
}

// Locker share-locks a version's providers and servers until the unit of work ends; a gone one answers *InvalidGrantError.
type Locker interface {
	LockShared(ctx context.Context, providerIDs, serverIDs []uuid.UUID) error
}
