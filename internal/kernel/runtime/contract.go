// Package runtime is the agent runtime feature: the turn contract Go and the runtime share, and the client that runs a turn.
package runtime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ContractVersion is the turn contract version; a breaking change bumps it, an optional field old peers refuse only when set does not.
const ContractVersion = 3

// TurnRequest is the body of POST /turn. SECURITY: it carries provider and MCP secrets; never log or trace it.
type TurnRequest struct {
	ContractVersion int               `json:"contractVersion"`
	IDs             TurnIDs           `json:"ids"`
	Agent           AgentSpec         `json:"agent"`
	MCPServers      []MCPServerSpec   `json:"mcpServers"`
	Snapshot        json.RawMessage   `json:"snapshot"`
	History         []json.RawMessage `json:"history"`
	Input           TurnInput         `json:"input"`
	StoredResults   []StoredResult    `json:"storedResults"`
	Limits          Limits            `json:"limits"`
	Knowledge       []Passage         `json:"knowledge,omitempty"`
}

// The runtime refuses a turn whose knowledge passes these bounds.
const (
	MaxPassages          = 10
	MaxPassageTextBytes  = 20000
	MaxPassageLabelBytes = 500
	MaxPassageURLBytes   = 2048
	// MaxKnowledgeTextBytes bounds the passage text of one turn, about 8,000 tokens; an agent's budget may be lower.
	MaxKnowledgeTextBytes = 32000
)

// UntitledPassage is the title NormalizePassages gives a passage that has none.
const UntitledPassage = "Untitled"

// NormalizePassages returns at most MaxPassages copies cut to the runtime's bounds and budgetBytes, dropping passages without text.
func NormalizePassages(in []Passage, budgetBytes int) []Passage {
	budget := min(budgetBytes, MaxKnowledgeTextBytes)
	out := make([]Passage, 0, min(len(in), MaxPassages))
	total := 0
	for _, p := range in {
		if len(out) == MaxPassages {
			break
		}
		n := Passage{
			Text:    cutUTF8(p.Text, MaxPassageTextBytes),
			Title:   cutUTF8(p.Title, MaxPassageLabelBytes),
			Heading: cutUTF8(p.Heading, MaxPassageLabelBytes),
		}
		if blank(n.Text) {
			continue
		}
		if blank(n.Title) {
			n.Title = UntitledPassage
		}
		if blank(n.Heading) {
			n.Heading = ""
		}
		if p.Page != nil && *p.Page >= 1 {
			n.Page = new(*p.Page)
		}
		if len(p.URL) <= MaxPassageURLBytes && utf8.ValidString(p.URL) {
			n.URL = p.URL
		}
		room := budget - total - len(n.Title) - len(n.Heading) - len(n.URL)
		if len(n.Text) > room {
			if len(out) > 0 || room <= 0 {
				break
			}
			n.Text = cutUTF8(n.Text, room)
		}
		total += len(n.Text) + len(n.Title) + len(n.Heading) + len(n.URL)
		out = append(out, n)
	}
	return out
}

func blank(s string) bool {
	return strings.TrimFunc(s, func(r rune) bool { return unicode.IsSpace(r) || unicode.Is(unicode.Cf, r) }) == ""
}

func cutUTF8(s string, maxBytes int) string {
	s = strings.ToValidUTF8(s, "\uFFFD")
	if len(s) <= maxBytes {
		return s
	}
	i := maxBytes
	for i > 0 && !utf8.RuneStart(s[i]) {
		i--
	}
	return s[:i]
}

// Passage is one retrieved knowledge passage with its source. SECURITY: project content; never log or store it.
type Passage struct {
	Text    string `json:"text"`
	Title   string `json:"title"`
	Heading string `json:"heading,omitempty"`
	Page    *int32 `json:"page,omitempty"`
	URL     string `json:"url,omitempty"`
}

// TurnIDs names the turn; Thread and Agent are "t" and "a" plus 32 lowercase hex, Project is a cache key only.
type TurnIDs struct {
	Run          string `json:"run"`
	Thread       string `json:"thread"`
	Agent        string `json:"agent"`
	AgentVersion int32  `json:"agentVersion"`
	Attempt      int32  `json:"attempt"`
	Project      string `json:"project"`
}

// AgentSpec is the agent config a turn runs with: a published version, or a draft for a Try it run.
type AgentSpec struct {
	Instructions string    `json:"instructions"`
	Temperature  float64   `json:"temperature"`
	MaxSteps     int32     `json:"maxSteps"`
	Model        ModelSpec `json:"model"`
}

// ProviderAnthropic is the ModelSpec provider of an Anthropic Messages API endpoint.
const ProviderAnthropic = "anthropic"

// ModelSpec is the model endpoint of a turn; an empty Provider means OpenAI-compatible.
type ModelSpec struct {
	Provider  string `json:"provider,omitempty"`
	BaseURL   string `json:"baseUrl"`
	APIKey    string `json:"apiKey"`
	ModelID   string `json:"modelId"`
	MaxTokens *int32 `json:"maxTokens,omitempty"`
}

// MCPServerSpec is one MCP server a turn may call; version 3 sends Tools with their approval specs, older versions AllowedTools.
type MCPServerSpec struct {
	Name             string      `json:"name"`
	URL              string      `json:"url"`
	Auth             MCPAuth     `json:"auth"`
	AllowedTools     []string    `json:"allowedTools,omitempty"`
	Tools            []ToolGrant `json:"tools,omitempty"`
	ConnectTimeoutMs int32       `json:"connectTimeoutMs"`
	CallTimeoutMs    int32       `json:"callTimeoutMs"`
}

// ToolGrant is one granted tool with its approval spec; contract version 3 sends it instead of AllowedTools.
type ToolGrant struct {
	Name     string       `json:"name"`
	Approval ApprovalSpec `json:"approval"`
}

// ApprovalSpec is when a person must approve a call: mode never, always or conditional with FreeWhen.
type ApprovalSpec struct {
	Mode     string          `json:"mode"`
	FreeWhen []FreeCondition `json:"freeWhen,omitempty"`
}

// FreeCondition lets a call run free when Arg equals one of Values, or is missing and Absent is set.
type FreeCondition struct {
	Arg    string            `json:"arg"`
	Values []json.RawMessage `json:"values"`
	Absent bool              `json:"absent,omitempty"`
}

// MCPAuth is how the runtime authenticates to an MCP server: kind none, bearer or header.
type MCPAuth struct {
	Kind   string `json:"kind"`
	Header string `json:"header,omitempty"`
	Value  string `json:"value,omitempty"`
}

// The turn input kinds.
const (
	InputMessage  = "message"
	InputResume   = "resume"
	InputContinue = "continue"
)

// TurnInput is a new user message (message), the answers to pending interrupts (resume), or nothing new after a crash (continue).
type TurnInput struct {
	Kind      string              `json:"kind"`
	Message   json.RawMessage     `json:"message,omitempty"`
	Responses []InterruptResponse `json:"responses,omitempty"`
}

// InterruptResponse answers one pending interrupt.
type InterruptResponse struct {
	InterruptID string          `json:"interruptId"`
	Response    json.RawMessage `json:"response"`
}

// StoredResult is a tool result {status, content} checkpointed earlier in the run; the runtime returns it instead of calling the tool.
type StoredResult struct {
	ToolUseID string          `json:"toolUseId"`
	Result    json.RawMessage `json:"result"`
}

// Limits bound a turn; a nil Budget means the agent has no cost limit.
type Limits struct {
	DeadlineMs int64   `json:"deadlineMs"`
	Budget     *Budget `json:"budget"`
}

// Budget is the money left for the run and the model's prices, all in micro-USD (prices per 1M tokens).
type Budget struct {
	RemainingMicros int64  `json:"remainingMicros"`
	Prices          Prices `json:"prices"`
}

// Prices are micro-USD per 1M tokens, per token class.
type Prices struct {
	Input      int64 `json:"input"`
	Output     int64 `json:"output"`
	CacheRead  int64 `json:"cacheRead"`
	CacheWrite int64 `json:"cacheWrite"`
}

// EventType names one line of the turn stream.
type EventType string

// The event types of the turn stream: the AG-UI events the runtime relays, then the private ones Go never relays.
const (
	EventRunStarted         EventType = "RUN_STARTED"
	EventTextMessageStart   EventType = "TEXT_MESSAGE_START"
	EventTextMessageContent EventType = "TEXT_MESSAGE_CONTENT"
	EventTextMessageEnd     EventType = "TEXT_MESSAGE_END"
	EventToolCallStart      EventType = "TOOL_CALL_START"
	EventToolCallArgs       EventType = "TOOL_CALL_ARGS"
	EventToolCallEnd        EventType = "TOOL_CALL_END"
	EventToolCallResult     EventType = "TOOL_CALL_RESULT"
	EventRunFinished        EventType = "RUN_FINISHED"
	EventRunError           EventType = "RUN_ERROR"
	EventHeartbeat          EventType = "OAGENT_HEARTBEAT"
	EventCheckpoint         EventType = "OAGENT_CHECKPOINT"
	EventTerminal           EventType = "OAGENT_TERMINAL"
)

// EventTypes returns every event type the contract allows.
func EventTypes() []EventType {
	return []EventType{
		EventRunStarted, EventTextMessageStart, EventTextMessageContent, EventTextMessageEnd,
		EventToolCallStart, EventToolCallArgs, EventToolCallEnd, EventToolCallResult,
		EventRunFinished, EventRunError, EventHeartbeat, EventCheckpoint, EventTerminal,
	}
}

// Event is one decoded line of the turn stream.
type Event interface {
	EventType() EventType
}

// RunStarted opens the turn's AG-UI stream.
type RunStarted struct {
	Type      EventType `json:"type"`
	Timestamp *int64    `json:"timestamp,omitempty"`
	ThreadID  string    `json:"threadId"`
	RunID     string    `json:"runId"`
}

// TextMessageStart opens an assistant text message.
type TextMessageStart struct {
	Type      EventType `json:"type"`
	Timestamp *int64    `json:"timestamp,omitempty"`
	MessageID string    `json:"messageId"`
	Role      string    `json:"role,omitempty"`
}

// TextMessageContent is one streamed chunk of a text message.
type TextMessageContent struct {
	Type      EventType `json:"type"`
	Timestamp *int64    `json:"timestamp,omitempty"`
	MessageID string    `json:"messageId"`
	Delta     string    `json:"delta"`
}

// TextMessageEnd closes a text message.
type TextMessageEnd struct {
	Type      EventType `json:"type"`
	Timestamp *int64    `json:"timestamp,omitempty"`
	MessageID string    `json:"messageId"`
}

// ToolCallStart opens a tool call; ToolCallName is the prefixed name the model called.
type ToolCallStart struct {
	Type            EventType `json:"type"`
	Timestamp       *int64    `json:"timestamp,omitempty"`
	ToolCallID      string    `json:"toolCallId"`
	ToolCallName    string    `json:"toolCallName"`
	ParentMessageID string    `json:"parentMessageId,omitempty"`
}

// ToolCallArgs is one streamed chunk of a tool call's JSON arguments.
type ToolCallArgs struct {
	Type       EventType `json:"type"`
	Timestamp  *int64    `json:"timestamp,omitempty"`
	ToolCallID string    `json:"toolCallId"`
	Delta      string    `json:"delta"`
}

// ToolCallEnd closes a tool call's arguments.
type ToolCallEnd struct {
	Type       EventType `json:"type"`
	Timestamp  *int64    `json:"timestamp,omitempty"`
	ToolCallID string    `json:"toolCallId"`
}

// ToolCallResult carries a tool's result back into the conversation.
type ToolCallResult struct {
	Type       EventType       `json:"type"`
	Timestamp  *int64          `json:"timestamp,omitempty"`
	MessageID  string          `json:"messageId"`
	ToolCallID string          `json:"toolCallId"`
	Content    json.RawMessage `json:"content"`
	Role       string          `json:"role,omitempty"`
}

// RunFinished closes the AG-UI stream of a turn that ended normally.
type RunFinished struct {
	Type      EventType       `json:"type"`
	Timestamp *int64          `json:"timestamp,omitempty"`
	ThreadID  string          `json:"threadId"`
	RunID     string          `json:"runId"`
	Result    json.RawMessage `json:"result,omitempty"`
	// Outcome is the AG-UI run outcome Go adds when the run waits on an approval; the runtime never sends it.
	Outcome json.RawMessage `json:"outcome,omitempty"`
}

// RunError closes the AG-UI stream of a turn that did not end normally.
type RunError struct {
	Type      EventType `json:"type"`
	Timestamp *int64    `json:"timestamp,omitempty"`
	Message   string    `json:"message"`
	Code      string    `json:"code,omitempty"`
}

// Heartbeat keeps an idle stream open; it carries nothing.
type Heartbeat struct {
	Type EventType `json:"type"`
}

// Checkpoint records one completed tool call, so a resumed turn never runs it again; Result is the Strands tool result {status, content}.
type Checkpoint struct {
	Type      EventType       `json:"type"`
	ToolUseID string          `json:"toolUseId"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	Result    json.RawMessage `json:"result"`
}

// Terminal is always the last line of a turn Go did not cut; Snapshot is null on failure, MessagesAppended are the turn's AG-UI messages.
type Terminal struct {
	Type             EventType         `json:"type"`
	Status           string            `json:"status"`
	StopReason       string            `json:"stopReason"`
	Snapshot         json.RawMessage   `json:"snapshot"`
	Usage            Usage             `json:"usage"`
	MessagesAppended []json.RawMessage `json:"messagesAppended"`
	Interrupts       []Interrupt       `json:"interrupts"`
	Notes            []Note            `json:"notes"`
	Error            *TerminalError    `json:"error,omitempty"`
}

// Usage is the tokens a turn used; Estimated is true when a cut model call was estimated.
type Usage struct {
	InputTokens           int64 `json:"inputTokens"`
	OutputTokens          int64 `json:"outputTokens"`
	CacheReadInputTokens  int64 `json:"cacheReadInputTokens"`
	CacheWriteInputTokens int64 `json:"cacheWriteInputTokens"`
	Estimated             bool  `json:"estimated"`
}

// Interrupt is one pending question the turn stopped on; an approval's Reason is {tool, input}.
type Interrupt struct {
	ID     string          `json:"id"`
	Name   string          `json:"name"`
	Reason json.RawMessage `json:"reason"`
}

// Note is something the turn wants recorded, such as an MCP server that was unavailable.
type Note struct {
	Kind   string `json:"kind"`
	Server string `json:"server,omitempty"`
}

// TerminalError is the stable cause of a turn that did not complete.
type TerminalError struct {
	Code string `json:"code"`
}

// The terminal statuses.
const (
	StatusCompleted        = "completed"
	StatusAwaitingApproval = "awaiting_approval"
	StatusLimitReached     = "limit_reached"
	StatusCancelled        = "cancelled"
	StatusFailed           = "failed"
)

//nolint:gochecknoglobals // a fixed list.
var terminalStatuses = []string{StatusCompleted, StatusAwaitingApproval, StatusLimitReached, StatusCancelled, StatusFailed}

func (e *RunStarted) EventType() EventType         { return EventRunStarted }
func (e *TextMessageStart) EventType() EventType   { return EventTextMessageStart }
func (e *TextMessageContent) EventType() EventType { return EventTextMessageContent }
func (e *TextMessageEnd) EventType() EventType     { return EventTextMessageEnd }
func (e *ToolCallStart) EventType() EventType      { return EventToolCallStart }
func (e *ToolCallArgs) EventType() EventType       { return EventToolCallArgs }
func (e *ToolCallEnd) EventType() EventType        { return EventToolCallEnd }
func (e *ToolCallResult) EventType() EventType     { return EventToolCallResult }
func (e *RunFinished) EventType() EventType        { return EventRunFinished }
func (e *RunError) EventType() EventType           { return EventRunError }
func (e *Heartbeat) EventType() EventType          { return EventHeartbeat }
func (e *Checkpoint) EventType() EventType         { return EventCheckpoint }
func (e *Terminal) EventType() EventType           { return EventTerminal }

// DecodeRequest decodes a turn request, refusing any field the contract does not name.
func DecodeRequest(raw []byte) (*TurnRequest, error) {
	var req TurnRequest
	if err := strict(raw, &req); err != nil {
		return nil, fmt.Errorf("runtime: decode request: %w", err)
	}
	return &req, nil
}

// DecodeEvent decodes one stream line into its event, refusing an unknown type or field.
func DecodeEvent(line []byte) (Event, error) {
	var head struct {
		Type EventType `json:"type"`
	}
	if err := json.Unmarshal(line, &head); err != nil {
		return nil, fmt.Errorf("runtime: decode event: %w", err)
	}
	ev := newEvent(head.Type)
	if ev == nil {
		return nil, fmt.Errorf("runtime: decode event: unknown type %q", head.Type)
	}
	if err := strict(line, ev); err != nil {
		return nil, fmt.Errorf("runtime: decode %s: %w", head.Type, err)
	}
	return ev, nil
}

func newEvent(t EventType) Event {
	switch t {
	case EventRunStarted:
		return &RunStarted{}
	case EventTextMessageStart:
		return &TextMessageStart{}
	case EventTextMessageContent:
		return &TextMessageContent{}
	case EventTextMessageEnd:
		return &TextMessageEnd{}
	case EventToolCallStart:
		return &ToolCallStart{}
	case EventToolCallArgs:
		return &ToolCallArgs{}
	case EventToolCallEnd:
		return &ToolCallEnd{}
	case EventToolCallResult:
		return &ToolCallResult{}
	case EventRunFinished:
		return &RunFinished{}
	case EventRunError:
		return &RunError{}
	case EventHeartbeat:
		return &Heartbeat{}
	case EventCheckpoint:
		return &Checkpoint{}
	case EventTerminal:
		return &Terminal{}
	}
	return nil
}

func strict(raw []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}
