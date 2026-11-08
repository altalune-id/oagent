package run

import (
	"context"

	"github.com/google/uuid"

	enduserv1 "altalune.id/oagent/gen/go/enduser/v1"
	"altalune.id/oagent/internal/kernel/events"
)

// SnapshotStore keeps one runtime snapshot per thread of the org on ctx; Load of a missing one is nil, nil.
type SnapshotStore interface {
	Load(ctx context.Context, threadID uuid.UUID) (*Snapshot, error)
	Save(ctx context.Context, snap *Snapshot) error
	Delete(ctx context.Context, threadID uuid.UUID) error
}

// AgentVersion is the agent config a run is pinned to: a published version, or a draft for a Try it run.
type AgentVersion struct {
	ID, AgentID   uuid.UUID
	AgentPublicID string
	// Version is 0 for a draft.
	Version      int32
	Draft        bool
	Legacy       bool
	Instructions string
	Model        string
	Temperature  float64
	MaxSteps     int32
	MaxCostCents int32
	ProviderID   uuid.UUID
	MCPServers   []MCPGrant
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

// Agents loads agent versions of the org on ctx.
type Agents interface {
	Version(ctx context.Context, versionID uuid.UUID) (*AgentVersion, error)
	// Latest returns the agent's newest published version, or nil before the first publish.
	Latest(ctx context.Context, agentID uuid.UUID) (*AgentVersion, error)
	// Draft pins the agent's draft config as a draft version, refused like a publish.
	Draft(ctx context.Context, agentID uuid.UUID) (*AgentVersion, error)
}

// ResolvedModel is how to call one model of a provider; nil Prices means unpriced. SECURITY: APIKey is an opened secret.
type ResolvedModel struct {
	BaseURL, APIKey string
	Prices          *Prices
	// Vision means the model reads images.
	Vision bool
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

// Refs turns project and agent public ids into ids and back for the thread handler.
type Refs interface {
	ProjectID(ctx context.Context, publicID string) (id, orgID uuid.UUID, ok bool, err error)
	ProjectPublicID(ctx context.Context, id uuid.UUID) (string, error)
	ResourceID(ctx context.Context, orgID uuid.UUID, publicID string) (uuid.UUID, bool, error)
	ResourcePublicID(ctx context.Context, orgID, id uuid.UUID) (publicID string, ok bool, err error)
}

// Media reads an inbound photo of a channel message and marks a message read, through the thread's connection.
type Media interface {
	Fetch(ctx context.Context, connectionID uuid.UUID, messageID, mime string, maxBytes int64) (data []byte, mimeType string, err error)
	MarkRead(ctx context.Context, connectionID uuid.UUID, messageID string) error
}

// Names gives an end user's display name for the speaker prefix of a group line; empty when it has none.
type Names interface {
	DisplayName(ctx context.Context, endUserID uuid.UUID) (string, error)
}

// Person shows an end user: a display name, a kind with a masked hint, and their one-line Label. SECURITY: never an identity value.
type Person struct {
	DisplayName string
	Kind        enduserv1.IdentityKind
	Hint, Label string
}

// People names end users for display; an unknown end user is the zero Person.
type People interface {
	Person(ctx context.Context, id uuid.UUID) (Person, error)
}
