package run

import (
	"encoding/json"
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
	CreatedAt                      time.Time
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
}
