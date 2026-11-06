package run

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Role is who wrote a transcript message.
type Role string

// The message roles.
const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
	RoleSystem    Role = "system"
)

// MessageStatus says whether a run has taken a message yet.
type MessageStatus string

// The message statuses.
const (
	MessagePending  MessageStatus = "pending"
	MessageConsumed MessageStatus = "consumed"
)

// Message is one transcript entry of a thread; Body is an AG-UI message object.
type Message struct {
	ID, OrgID, ProjectID, ThreadID uuid.UUID
	Seq                            int64
	Role                           Role
	Body                           json.RawMessage
	SpeakerID                      *uuid.UUID
	Status                         MessageStatus
	ExternalMessageID              string
	RunID                          *uuid.UUID
	ToolCallID                     string
	// RequestHash is the SHA-256 of an idempotent request's canonical body; nil for every other message.
	RequestHash []byte
	CreatedAt   time.Time
}

// Snapshot is the runtime's opaque state of a thread at a turn boundary.
type Snapshot struct {
	ThreadID, OrgID, ProjectID, AgentVersionID uuid.UUID
	SDKVersion                                 string
	Blob                                       []byte
	UpdatedAt                                  time.Time
}

// StoredResult is a checkpointed tool result of a run: the AG-UI tool message stored for ToolUseID.
type StoredResult struct {
	ToolUseID string
	Body      json.RawMessage
}

// Thread is one conversation of an agent; ExternalKey names it on its channel.
type Thread struct {
	ID, OrgID, ProjectID, AgentID uuid.UUID
	PublicID, Kind, ExternalKey   string
	CancelRequested               bool
	// ActorEndUserID is the end user an API thread is bound to; uuid.Nil when unbound or deleted.
	ActorEndUserID uuid.UUID
}

const threadKindAPI = "api"

// ClaimResult says whether a claim won the lease, and why not.
type ClaimResult int

// The claim results.
const (
	// LeaseLive means another turn holds the thread, or the thread is not visible to the caller.
	LeaseLive ClaimResult = iota
	// ProjectBusy means the thread is free but its project is at its concurrency cap.
	ProjectBusy
	// Claimed means the lease is the caller's.
	Claimed
)

// IdempotencyPrefix marks a message's external id as the Idempotency-Key of the POST that sent it.
const IdempotencyPrefix = "idem:"

// IdempotencyKeyOf returns the Idempotency-Key of the first message in msgs that came with one.
func IdempotencyKeyOf(msgs []Message) string {
	for _, m := range msgs {
		if key, ok := strings.CutPrefix(m.ExternalMessageID, IdempotencyPrefix); ok {
			return key
		}
	}
	return ""
}

// Ref is what a thread or run public id resolves to, read before any tenant-scoped read for the reach check; AgentID is the thread's.
type Ref struct{ ID, OrgID, ProjectID, AgentID uuid.UUID }

// Cursor marks where a newest-first page ended; the zero value starts at the newest.
type Cursor struct {
	At       time.Time
	PublicID string
}

// ThreadSummary is a thread with its last activity and open run status, empty when none; only ThreadSummary counts Messages and Runs.
type ThreadSummary struct {
	Thread
	LastActivityAt time.Time
	OpenRun        Status
	Messages, Runs int64
}

// AGUIBody is the message's AG-UI fields only. SECURITY: Go's own bookkeeping, such as a checkpoint's raw result, stays server-side.
func (m Message) AGUIBody() json.RawMessage {
	var all map[string]json.RawMessage
	if json.Unmarshal(m.Body, &all) != nil {
		return json.RawMessage(`{}`)
	}
	kept := map[string]json.RawMessage{}
	for _, k := range []string{"id", "role", "content", "name", "toolCalls", "toolCallId", "error"} {
		if v, ok := all[k]; ok {
			kept[k] = v
		}
	}
	out, err := json.Marshal(kept)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return out
}
