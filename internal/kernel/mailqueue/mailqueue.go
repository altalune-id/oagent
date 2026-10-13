// Package mailqueue is the durable mail queue on Postgres: messages are sealed at rest and erased once settled.
package mailqueue

import (
	"time"

	"github.com/google/uuid"
)

// Status is the lifecycle state of a queued message.
type Status string

// Entry states, of which StatusPending is the only non-terminal one.
const (
	StatusPending Status = "pending"
	StatusSent    Status = "sent"
	StatusFailed  Status = "failed"
)

const (
	// MaxAttempts bounds send tries; an entry that reaches it moves to StatusFailed.
	MaxAttempts = 8
	// ClaimLease is how far ClaimDue pushes next_attempt_at, releasing an entry whose sender died.
	ClaimLease = 5 * time.Minute
	// MaxClaimLimit caps one ClaimDue batch.
	MaxClaimLimit = 1000
	// MaxErrorLen bounds the failure text kept on an entry.
	MaxErrorLen = 512
	// Retention is how long a settled entry, which no longer holds its message, is kept.
	Retention = 30 * 24 * time.Hour
)

// Entry is one queued message as the worker sees it; Payload is sealed unless Sealed is false.
type Entry struct {
	ID            uuid.UUID
	Payload       []byte
	Sealed        bool
	Status        Status
	Attempt       int
	NextAttemptAt time.Time
	LastError     string
	CreatedAt     time.Time
}

func claimBatch(limit int) int64 { return int64(min(max(limit, 1), MaxClaimLimit)) }

type entryRow struct {
	ID            uuid.UUID `alias:"mail_queue.id"`
	Payload       []byte    `alias:"mail_queue.payload"`
	Sealed        bool      `alias:"mail_queue.sealed"`
	Status        string    `alias:"mail_queue.status"`
	Attempt       int32     `alias:"mail_queue.attempt"`
	NextAttemptAt time.Time `alias:"mail_queue.next_attempt_at"`
	LastError     string    `alias:"mail_queue.last_error"`
	CreatedAt     time.Time `alias:"mail_queue.created_at"`
}

func (r *entryRow) toEntry() Entry {
	return Entry{
		ID:            r.ID,
		Payload:       r.Payload,
		Sealed:        r.Sealed,
		Status:        Status(r.Status),
		Attempt:       int(r.Attempt),
		NextAttemptAt: r.NextAttemptAt.UTC(),
		LastError:     r.LastError,
		CreatedAt:     r.CreatedAt.UTC(),
	}
}
