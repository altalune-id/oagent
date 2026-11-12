package approval

import (
	"encoding/json"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// Status is where an approval stands.
type Status string

// The approval statuses; pending is the only one a decision changes.
const (
	StatusPending  Status = "pending"
	StatusApproved Status = "approved"
	StatusRejected Status = "rejected"
	StatusExpired  Status = "expired"
	// StatusDecided is a list filter only: approved or rejected.
	StatusDecided Status = "decided"
)

// CallState is how far an approved call got.
type CallState string

// The call states after a decision.
const (
	CallApproved  CallState = "approved"
	CallExecuting CallState = "executing"
	CallExecuted  CallState = "executed"
	CallUnknown   CallState = "unknown"
)

// Channel is where an approval was raised.
type Channel string

// The channels an approval comes from.
const (
	ChannelWhatsApp Channel = "whatsapp"
	ChannelAPI      Channel = "api"
	ChannelConsole  Channel = "console"
)

// Via is where a decision came from.
type Via string

// The places a decision comes from.
const (
	ViaConsole  Via = "console"
	ViaMCP      Via = "mcp"
	ViaWhatsApp Via = "whatsapp"
	ViaAPI      Via = "api"
)

// DeciderKind is who decided an approval.
type DeciderKind string

// The kinds of decider.
const (
	DeciderEndUser DeciderKind = "end_user"
	DeciderAPIKey  DeciderKind = "api_key"
	DeciderUser    DeciderKind = "user"
	DeciderSystem  DeciderKind = "system"
)

const (
	maxCalls        = 8
	maxDecisionText = 4000
	groupExpiry     = 10 * time.Minute
	defaultExpiry   = 24 * time.Hour
)

// Call is one gated tool call the runtime raised; Keys is its arguments' order in the live schema.
type Call struct {
	ToolCallID  string          `json:"tool_call_id"`
	InterruptID string          `json:"interrupt_id"`
	Server      string          `json:"server"`
	Tool        string          `json:"tool"`
	Args        json.RawMessage `json:"args"`
	ArgsHash    string          `json:"args_hash"`
	Keys        []string        `json:"keys,omitempty"`
	State       CallState       `json:"state,omitempty"`
}

// Approval holds a run's gated calls for one decision; it is single use.
type Approval struct {
	ID, OrgID, ProjectID, AgentID, RunID, ThreadID uuid.UUID
	PublicID                                       string
	Calls                                          []Call
	Preview                                        string
	Channel                                        Channel
	Requester                                      uuid.NullUUID
	PromptExternalID                               string
	// PromptAfterSeq is the thread's last message seq when the prompt was sent; nil until then. Only later messages may decide.
	PromptAfterSeq       *int64
	RaisedSnapshotHash   []byte
	Status               Status
	DecidedByKind        DeciderKind
	DecidedByID          string
	DecidedVia           Via
	DecisionText         string
	ExpiresAt, CreatedAt time.Time
	DecidedAt            *time.Time
}

// Tools lists the gated calls' tool names in call order.
func (a *Approval) Tools() []string {
	out := make([]string, 0, len(a.Calls))
	for _, c := range a.Calls {
		out = append(out, c.Tool)
	}
	return out
}

// ChannelOf names the channel of a thread kind: a DM or group is WhatsApp, a try thread the console, the rest the API.
func ChannelOf(threadKind string) Channel {
	switch threadKind {
	case "dm", "group":
		return ChannelWhatsApp
	case "try":
		return ChannelConsole
	}
	return ChannelAPI
}

// ExpiryOf is how long an approval raised in a thread of threadKind waits: 10 minutes in a group, 24 hours elsewhere.
func ExpiryOf(threadKind string) time.Duration {
	if threadKind == "group" {
		return groupExpiry
	}
	return defaultExpiry
}

// RequesterOf is the speaker of the turn's last taken message, the one a group reply quotes; nil when nobody spoke.
func RequesterOf(speakers []uuid.NullUUID) uuid.NullUUID {
	if len(speakers) == 0 {
		return uuid.NullUUID{}
	}
	return speakers[len(speakers)-1]
}

func capRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}
