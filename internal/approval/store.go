package approval

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Filter narrows a project's approvals; an empty AgentIDs or Status matches any, and pending means unexpired at Now.
type Filter struct {
	ProjectID uuid.UUID
	AgentIDs  []uuid.UUID
	Status    Status
	Now       time.Time
}

// Ref names an approval's ids and where it lives, resolved without a tenant.
type Ref struct {
	ID, OrgID, ProjectID, AgentID uuid.UUID
}

// Cursor is the last row of a page, newest first.
type Cursor struct {
	At       time.Time
	PublicID string
}

// Decided is the outcome a decision writes.
type Decided struct {
	Status Status
	By     Decider
	Via    Via
	Text   string
	At     time.Time
}

// Store persists approvals for the org bound in ctx.
type Store interface {
	Insert(ctx context.Context, a *Approval) error
	Get(ctx context.Context, id uuid.UUID) (*Approval, error)
	List(ctx context.Context, f Filter, after Cursor, limit int) ([]*Approval, error)
	// CountPending counts a project's pending approvals unexpired at now, of the given agents or of every agent when agents is empty.
	CountPending(ctx context.Context, projectID uuid.UUID, agents []uuid.UUID, now time.Time) (int, error)
	// Resolve finds an approval by public id in any org; the caller checks reach before it reads the approval.
	Resolve(ctx context.Context, publicID string) (Ref, error)
	PendingForThread(ctx context.Context, threadID uuid.UUID) (*Approval, error)
	// LatestForRun is the run's newest approval, the only one a resume answers.
	LatestForRun(ctx context.Context, runID uuid.UUID) (*Approval, error)
	// Decide writes d only while the approval is pending and unexpired at d.At; ok is false when 0 rows matched.
	Decide(ctx context.Context, id uuid.UUID, d Decided) (a *Approval, ok bool, err error)
	// SetCallStates moves the named calls from one state to another; a call in any other state is an error.
	SetCallStates(ctx context.Context, id uuid.UUID, toolCallIDs []string, from, to CallState) error
	// ExpireUnresumable marks id expired when it is pending or approved, keeping a person's decider and time; ok is false otherwise.
	ExpireUnresumable(ctx context.Context, id uuid.UUID) (a *Approval, ok bool, err error)
	// DueExpiries lists pending approvals of the bound org past their expiry on the database clock, oldest first, at most limit.
	DueExpiries(ctx context.Context, limit int) ([]uuid.UUID, error)
	// Expire marks id expired by the system when it is pending and past its expiry on the database clock; ok is false otherwise.
	Expire(ctx context.Context, id uuid.UUID) (a *Approval, ok bool, err error)
	// SetPromptSent records the sent prompt's provider id (never an empty one) and the thread's last seq then; first values stay.
	SetPromptSent(ctx context.Context, id uuid.UUID, externalID string, afterSeq int64) error
}
