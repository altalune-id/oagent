// Package run models one agent turn on a thread: its status, attempts, usage and cost.
package run

import (
	"slices"
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/nanoid"
)

// Status is where a run is in its life.
type Status string

// The run statuses.
const (
	StatusRunning          Status = "running"
	StatusAwaitingApproval Status = "awaiting_approval"
	StatusCompleted        Status = "completed"
	StatusFailed           Status = "failed"
	StatusCancelled        Status = "cancelled"
	StatusInterrupted      Status = "interrupted"
	StatusLimitReached     Status = "limit_reached"
	StatusExpired          Status = "expired"
)

// Statuses returns every run status.
func Statuses() []Status {
	return []Status{
		StatusRunning, StatusAwaitingApproval, StatusCompleted, StatusFailed,
		StatusCancelled, StatusInterrupted, StatusLimitReached, StatusExpired,
	}
}

//nolint:gochecknoglobals // the fixed table of legal moves.
var moves = map[Status][]Status{
	StatusRunning: {
		StatusCompleted, StatusLimitReached, StatusFailed, StatusCancelled, StatusInterrupted, StatusAwaitingApproval, StatusExpired,
	},
	StatusInterrupted:      {StatusRunning, StatusFailed, StatusCancelled},
	StatusAwaitingApproval: {StatusRunning, StatusExpired, StatusCancelled},
}

// Terminal reports whether s is final: no move leaves it.
func (s Status) Terminal() bool { return len(moves[s]) == 0 }

// Usage is the tokens a run used, per class; Estimated is true when part of it was estimated.
type Usage struct {
	InputTokens      int64
	OutputTokens     int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	EmbeddingTokens  int64
	Estimated        bool
}

// Run is the aggregate root: one turn of an agent version on a thread.
type Run struct {
	ID, OrgID, ProjectID, ThreadID, AgentVersionID uuid.UUID
	PublicID                                       string
	Status                                         Status
	Attempt                                        int32
	Usage                                          Usage
	// Prices is the price snapshot taken when the run started; nil means unpriced.
	Prices *Prices
	// KnowledgeRetrieveCalls and KnowledgeRetrieveMs add up the run's knowledge searches over its legs.
	KnowledgeRetrieveCalls int32
	KnowledgeRetrieveMs    int64
	// KnowledgePassages sums the passages sent over the run's legs, a resume included; a leg that crashes before it settles adds none.
	KnowledgePassages int32
	// IdempotencyKey is the caller's key for the request that started the run; empty means none.
	IdempotencyKey string
	// ErrorCode is the apperror code of a failed run; empty otherwise.
	ErrorCode string
	// Group marks a run of a group thread: no single person, so no memories reach it.
	Group bool
	// ReplyState is a channel run's reply: ReplyDue, ReplySent, ReplyFailed or ReplyNone; empty for an API or unsettled run.
	ReplyState string
	// ReplyKind is what a due channel reply sends: ReplyKindPrompt for an approval prompt, else empty (the run's reply).
	ReplyKind string
	// NoReplyReason says why a completed or limit-reached channel run sent no reply, one of the NoReply reasons; empty otherwise.
	NoReplyReason        string
	StartedAt, UpdatedAt time.Time
	EndedAt              *time.Time
}

// New starts a running run, attempt 1, of agentVersionID on threadID at now.
func New(orgID, projectID, threadID, agentVersionID uuid.UUID, now time.Time) *Run {
	now = now.UTC()
	return &Run{
		ID: uuid.Must(uuid.NewV7()), PublicID: nanoid.MustNew(21), OrgID: orgID, ProjectID: projectID,
		ThreadID: threadID, AgentVersionID: agentVersionID, Status: StatusRunning, Attempt: 1,
		StartedAt: now, UpdatedAt: now,
	}
}

// Move changes the status at now if legal; resuming from interrupted is the next attempt, a terminal move records the end.
func (r *Run) Move(to Status, now time.Time) error {
	if !slices.Contains(moves[r.Status], to) {
		return &IllegalMoveError{From: r.Status, To: to}
	}
	if r.Status == StatusInterrupted && to == StatusRunning {
		r.Attempt++
	}
	r.Status, r.UpdatedAt = to, now.UTC()
	if to.Terminal() {
		end := r.UpdatedAt
		r.EndedAt = &end
	}
	return nil
}

const (
	// ReplyKindPrompt marks a due reply that is the approval prompt, which the sender builds from the approval.
	ReplyKindPrompt = "prompt"
	// ReplyKindExpired marks a due reply that tells the requester their approval expired.
	ReplyKindExpired = "expired"
)
