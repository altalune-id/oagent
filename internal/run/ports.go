package run

import (
	"context"
	"encoding/json"
	"time"

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
	// FreeWhen is the tool's free_when conditions as JSON, empty when there are none.
	FreeWhen json.RawMessage
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
	// Provider is the runtime.ModelSpec provider; empty means OpenAI-compatible.
	Provider        string
	BaseURL, APIKey string
	Prices          *Prices
	// Vision means the model reads images.
	Vision bool
}

// KnowledgeSource is one knowledge source an agent version searches.
type KnowledgeSource struct {
	ID       uuid.UUID
	PublicID string
}

// KnowledgeBindings lists the knowledge sources an agent version is bound to; none is an empty list.
type KnowledgeBindings interface {
	BoundSources(ctx context.Context, versionID uuid.UUID) ([]KnowledgeSource, error)
}

// KnowledgeQuery is one search before a turn: the turn's text against the version's sources in its project.
type KnowledgeQuery struct {
	ProjectID uuid.UUID
	Sources   []uuid.UUID
	Text      string
}

// KnowledgeHit is one passage a search returned, best first. SECURITY: project content; never log it.
type KnowledgeHit struct {
	Text, Title, Heading, URL string
	Page                      int
	Score                     float64
	Source                    uuid.UUID
}

// KnowledgeUsage is what one search spent.
type KnowledgeUsage struct {
	EmbeddingTokens int64
	RetrieveCalls   int
}

// KnowledgeRetriever searches a project's sources and returns at most the retriever's default number of hits.
type KnowledgeRetriever interface {
	Search(ctx context.Context, q KnowledgeQuery) ([]KnowledgeHit, KnowledgeUsage, error)
}

// EmbeddingPrices gives the input price of a project's embedding model, nil when the project has none.
type EmbeddingPrices interface {
	EmbeddingPrice(ctx context.Context, projectID uuid.UUID) (*int64, error)
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

// ApprovalCall is one gated tool call an approval interrupt raised; Keys is its arguments' order in the live schema.
type ApprovalCall struct {
	// State is empty while pending, then approved, executing, executed or unknown.
	State                                 string
	ToolCallID, InterruptID, Server, Tool string
	Args                                  json.RawMessage
	ArgsHash                              string
	Keys                                  []string
}

// RaiseApproval is the one approval an awaiting settle records for every approval interrupt of the step.
type RaiseApproval struct {
	OrgID, ProjectID, AgentID, RunID, ThreadID uuid.UUID
	ThreadKind                                 string
	Calls                                      []ApprovalCall
	Speakers                                   []uuid.NullUUID
	SnapshotHash                               []byte
}

// RunApproval is a run's latest approval as a resume reads it; Status is pending, approved, rejected or expired.
type RunApproval struct {
	ID                 uuid.UUID
	PublicID, Preview  string
	Status             string
	DecisionText       string
	ExpiresAt          time.Time
	Calls              []ApprovalCall
	RaisedSnapshotHash []byte
	// Channel is whatsapp, api or console; only a whatsapp approval is decided from the chat.
	Channel string
	// Requester is the end user whose message raised it; PromptExternalID is the sent prompt's provider id.
	Requester        uuid.NullUUID
	PromptExternalID string
	// PromptAfterSeq is the thread's last seq when the prompt was sent, nil until then; only later messages may decide.
	PromptAfterSeq *int64
}

// ChatAnswer is a requester's chat message deciding approval ApprovalID.
type ChatAnswer struct {
	ApprovalID, By uuid.UUID
	Approved       bool
	Text           string
}

// Approvals raises, reads and decides the approvals a run waits on; the approval service implements it.
type Approvals interface {
	Raise(ctx context.Context, in RaiseApproval) error
	Latest(ctx context.Context, runID uuid.UUID) (*RunApproval, error)
	// MatchWord reads a chat reply as yes or no; matched is false for any other text.
	MatchWord(text string) (approved, matched bool)
	// Expire marks approval id expired when it is still pending and past its expiry on the database clock.
	Expire(ctx context.Context, id uuid.UUID) (expired bool, err error)
	// NeedsApproval runs the approval evaluator on a call of grant with args, without a live schema.
	NeedsApproval(grant ToolGrant, args json.RawMessage) bool
	// MarkExecuting moves approved calls to executing; the caller commits it under the lease before the resume is sent.
	MarkExecuting(ctx context.Context, id uuid.UUID, toolCallIDs []string) error
	// MarkUndelivered moves executing calls back to approved when their request never reached the runtime.
	MarkUndelivered(ctx context.Context, id uuid.UUID, toolCallIDs []string) error
	// MarkExecuted records that an executing call's result was stored.
	MarkExecuted(ctx context.Context, id uuid.UUID, toolCallID string) error
	// MarkUnknown records that an executing call has no stored result after a restart, so it never runs again.
	MarkUnknown(ctx context.Context, id uuid.UUID, toolCallID string) error
	// ExpireUnresumable expires approval id with its run when the interrupt can't resume.
	ExpireUnresumable(ctx context.Context, id uuid.UUID) error
	// RejectOnCancel rejects pending approval id for the caller who cancels its run, or for the system when bySystem.
	RejectOnCancel(ctx context.Context, id uuid.UUID, bySystem bool) error
	// DecideFromChat records a WhatsApp decision; decided is false when the approval was no longer pending.
	DecideFromChat(ctx context.Context, a ChatAnswer) (decided bool, err error)
}
