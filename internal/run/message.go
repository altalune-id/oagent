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
	ChannelTime *time.Time
	SkipReason  string
	CreatedAt   time.Time
}

const (
	senderWindow    = time.Minute
	turnMaxMessages = 20
	turnMaxBytes    = 32 << 10
	maxMessageRunes = 4000
	turnMaxImages   = 3
	maxImageBytes   = 5 << 20
	notePhotoUnread = "[photo not read]"
	notePhoto       = "[photo]"
)

// DefaultMaxMessageAge is how old a channel message may be when its turn takes it before it is skipped as stale.
const DefaultMaxMessageAge = 30 * time.Minute

// The reply states of a channel run.
const (
	ReplyDue    = "due"
	ReplySent   = "sent"
	ReplyFailed = "failed"
	ReplyNone   = "none"
)

// The reasons a completed or limit-reached channel run sent no reply.
const (
	// NoReplyNoText means the turn ended without assistant text.
	NoReplyNoText = "no_text"
	// NoReplySetReplyEmpty means the set reply the run needed is empty on its connection.
	NoReplySetReplyEmpty = "set_reply_empty"
	// NoReplyStepLimit means the run stopped at its step limit before the model wrote any text.
	NoReplyStepLimit = "step_limit"
)

const replyGrace = 30 * time.Second

// ReplyJob is what sending a channel run's reply needs. SECURITY: To is a phone, LID or group address; never log it.
type ReplyJob struct {
	RunID                   uuid.UUID
	RunPublicID             string
	ConnectionID            uuid.UUID
	Kind, To, QuoteID, Text string
	State                   string
	// LastSeq is the thread's last message seq when the job was read; a prompt sent from it is answered only by later messages.
	LastSeq int64
	// ReplyKind is ReplyKindPrompt when the due reply is the approval prompt, else empty.
	ReplyKind           string
	PartsSent, Attempts int
	EndedAt, NextAt     time.Time
}

// ReplyTexts are a channel connection's set replies.
type ReplyTexts struct{ Failure, Voice, PhotoUnreadable string }

// SkipStale marks a channel message too old to answer when its turn took it; no run owns it.
const SkipStale = "stale"

// SkipSenderRate marks a channel message stored past its sender's per-minute cap; no turn takes it.
const SkipSenderRate = "sender_rate"

// ChannelMessage is one inbound channel message for the thread its connection and chat key name.
type ChannelMessage struct {
	ProjectID, AgentID, ConnectionID        uuid.UUID
	ChatKey, Kind, ChannelAddress, ReplyVia string
	// Actor is the person of a DM thread; uuid.Nil for a group.
	Actor     uuid.UUID
	Message   Message
	SenderCap int
}

// ChannelAppend says where a channel message landed: Pending when a turn should take it, Duplicate when it was already stored.
type ChannelAppend struct {
	ThreadID           uuid.UUID
	Pending, Duplicate bool
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
	// ConnectionID is a channel thread's connection; uuid.Nil for an API thread or once the connection is deleted.
	ConnectionID uuid.UUID
}

const (
	threadKindAPI   = "api"
	threadKindDM    = "dm"
	threadKindGroup = "group"
	threadKindTry   = "try"
)

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

// ThreadFilter narrows ListThreads; a zero field keeps every agent or kind.
type ThreadFilter struct {
	AgentID uuid.UUID
	Kind    string
}

// Cursor marks where a newest-first page ended; the zero value starts at the newest.
type Cursor struct {
	At       time.Time
	PublicID string
}

// EndUserActivity is when an end user last wrote and how many threads they own or wrote in.
type EndUserActivity struct {
	LastSeen time.Time
	Threads  int64
}

// ThreadSummary is a thread with its last activity and open run status, empty when none; only ThreadSummary counts Messages and Runs.
type ThreadSummary struct {
	Thread
	LastActivityAt time.Time
	OpenRun        Status
	Messages, Runs int64
}

// QuotedID is the provider id of the message this channel message quoted, empty when none.
func (m Message) QuotedID() string {
	var b struct {
		Channel struct {
			Quoted string `json:"quoted"`
		} `json:"channel"`
	}
	_ = json.Unmarshal(m.Body, &b)
	return b.Channel.Quoted
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
