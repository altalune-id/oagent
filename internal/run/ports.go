package run

import (
	"context"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/events"
)

// SnapshotStore keeps one runtime snapshot per thread of the org on ctx; Load of a missing one is nil, nil.
type SnapshotStore interface {
	Load(ctx context.Context, threadID uuid.UUID) (*Snapshot, error)
	Save(ctx context.Context, snap *Snapshot) error
	Delete(ctx context.Context, threadID uuid.UUID) error
}

// AgentVersion is the published agent config a run is pinned to.
type AgentVersion struct {
	ID, AgentID   uuid.UUID
	AgentPublicID string
	Version       int32
	Legacy        bool
	Instructions  string
	Model         string
	Temperature   float64
	MaxSteps      int32
	MaxCostCents  int32
	ProviderID    uuid.UUID
	MCPServers    []MCPGrant
}

// MCPGrant is the tools of one MCP server a version may call.
type MCPGrant struct {
	ServerID uuid.UUID
	Tools    []ToolGrant
}

// ToolGrant is one granted tool; RequireApproval makes a person approve each call.
type ToolGrant struct {
	Name            string
	RequireApproval bool
}

// Agents loads published agent versions of the org on ctx.
type Agents interface {
	Version(ctx context.Context, versionID uuid.UUID) (*AgentVersion, error)
	// Latest returns the agent's newest published version, or nil before the first publish.
	Latest(ctx context.Context, agentID uuid.UUID) (*AgentVersion, error)
}

// ResolvedModel is how to call one model of a provider; nil Prices means unpriced. SECURITY: APIKey is an opened secret.
type ResolvedModel struct {
	BaseURL, APIKey string
	Prices          *Prices
}

// Providers resolves a provider's model with its opened key.
type Providers interface {
	ResolveModel(ctx context.Context, providerID uuid.UUID, model string) (ResolvedModel, error)
}

// OpenedServer is an MCP server with its opened credential. SECURITY: AuthValue is an opened secret.
type OpenedServer struct {
	Name, URL                       string
	AuthKind, AuthHeader, AuthValue string
	TimeoutMS                       int32
}

// MCPServers opens a project's MCP server for a turn.
type MCPServers interface {
	Open(ctx context.Context, serverID uuid.UUID) (OpenedServer, error)
}

// Publisher fans a domain event out to its subscribers; the webhook module implements it.
type Publisher interface {
	Enqueue(ctx context.Context, t events.Type, data any) error
}
