// Package outbox is the durable, tenant-scoped delivery queue on Postgres.
package outbox

import (
	"math/rand/v2"
	"time"

	"github.com/google/uuid"
)

// Status is the lifecycle state of an outbox entry.
type Status string

// Entry states, of which StatusPending is the only non-terminal one.
const (
	StatusPending   Status = "pending"
	StatusDelivered Status = "delivered"
	StatusFailed    Status = "failed"
)

// Valid reports whether s is one of the three known states.
func (s Status) Valid() bool {
	return s == StatusPending || s == StatusDelivered || s == StatusFailed
}

// Terminal reports whether s admits no further transition.
func (s Status) Terminal() bool { return s == StatusDelivered || s == StatusFailed }

const (
	// MaxAttempts bounds delivery tries; an entry that reaches it moves to StatusFailed.
	MaxAttempts = 8
	// ClaimLease is how far ClaimDue pushes next_attempt_at, releasing an entry whose dispatcher died.
	ClaimLease = 5 * time.Minute
	// MaxTargetLen bounds the target identifier a caller may enqueue.
	MaxTargetLen = 255
	// MaxClaimLimit caps one ClaimDue batch.
	MaxClaimLimit = 1000
	// MaxListLimit caps one ListByTarget call.
	MaxListLimit = 100
)

// Entry is one durable delivery record.
type Entry struct {
	ID            uuid.UUID
	PublicID      string
	EventID       uuid.UUID
	EventPublicID string
	OrgID         uuid.UUID
	ProjectID     uuid.UUID
	Target        string
	Payload       []byte
	Attempt       int
	NextAttemptAt time.Time
	Status        Status
	LastError     string
	CreatedAt     time.Time
}

// BackoffBase returns the unjittered wait before the given attempt.
func BackoffBase(attempt int) time.Duration {
	switch {
	case attempt <= 2:
		return 30 * time.Second
	case attempt == 3:
		return 5 * time.Minute
	case attempt == 4:
		return 30 * time.Minute
	case attempt == 5:
		return 2 * time.Hour
	case attempt == 6:
		return 5 * time.Hour
	default:
		return 10 * time.Hour
	}
}

// Backoff returns the jittered wait before the given attempt.
func Backoff(attempt int) time.Duration {
	base := BackoffBase(attempt)
	spread := base / 10
	return base - spread + rand.N(2*spread+1)
}

func (e Entry) validate() error {
	switch {
	case e.ID == uuid.Nil:
		return &InvalidEntryError{Field: "ID", Reason: "must not be the zero uuid"}
	case e.PublicID == "":
		return &InvalidEntryError{Field: "PublicID", Reason: "must not be empty"}
	case e.EventID == uuid.Nil:
		return &InvalidEntryError{Field: "EventID", Reason: "must not be the zero uuid"}
	case e.EventPublicID == "":
		return &InvalidEntryError{Field: "EventPublicID", Reason: "must not be empty"}
	case e.OrgID == uuid.Nil:
		return &InvalidEntryError{Field: "OrgID", Reason: "must not be the zero uuid"}
	case e.ProjectID == uuid.Nil:
		return &InvalidEntryError{Field: "ProjectID", Reason: "must not be the zero uuid"}
	case e.Target == "":
		return &InvalidEntryError{Field: "Target", Reason: "must not be empty"}
	case len(e.Target) > MaxTargetLen:
		return &InvalidEntryError{Field: "Target", Reason: "exceeds the length limit"}
	case e.Status != "" && e.Status != StatusPending:
		return &InvalidEntryError{Field: "Status", Reason: "a new entry is always pending"}
	}
	return nil
}

func clampLimit(limit, limitCap int) int64 {
	return int64(min(max(limit, 1), limitCap))
}

func claimBatch(limit int) int64 { return clampLimit(limit, MaxClaimLimit) }

func listBatch(limit int) int64 { return clampLimit(limit, MaxListLimit) }

type entryRow struct {
	ID            uuid.UUID `alias:"outbox_entries.id"`
	EventID       uuid.UUID `alias:"outbox_entries.event_id"`
	OrgID         uuid.UUID `alias:"outbox_entries.org_id"`
	ProjectID     uuid.UUID `alias:"outbox_entries.project_id"`
	Target        string    `alias:"outbox_entries.target"`
	Payload       []byte    `alias:"outbox_entries.payload"`
	Attempt       int32     `alias:"outbox_entries.attempt"`
	NextAttemptAt time.Time `alias:"outbox_entries.next_attempt_at"`
	Status        string    `alias:"outbox_entries.status"`
	LastError     string    `alias:"outbox_entries.last_error"`
	CreatedAt     time.Time `alias:"outbox_entries.created_at"`
	PublicID      string    `alias:"outbox_entries.public_id"`
	EventPublicID string    `alias:"outbox_entries.event_public_id"`
}

func (r *entryRow) toEntry() Entry {
	return Entry{
		ID:            r.ID,
		PublicID:      r.PublicID,
		EventID:       r.EventID,
		EventPublicID: r.EventPublicID,
		OrgID:         r.OrgID,
		ProjectID:     r.ProjectID,
		Target:        r.Target,
		Payload:       r.Payload,
		Attempt:       int(r.Attempt),
		NextAttemptAt: r.NextAttemptAt.UTC(),
		Status:        Status(r.Status),
		LastError:     r.LastError,
		CreatedAt:     r.CreatedAt.UTC(),
	}
}
