package outbox

import (
	"context"
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/apperror"
)

// Store is the driven port for durable outbox persistence.
type Store interface {
	// Enqueue records a pending entry, ignoring a repeat of an (EventID, Target) already queued.
	Enqueue(ctx context.Context, e Entry) error
	// ClaimDue takes up to limit due entries exclusively, bumping Attempt and leasing each for ClaimLease.
	ClaimDue(ctx context.Context, now time.Time, limit int) ([]Entry, error)
	// Succeed settles the claimed entry e as delivered, refusing a settle from a superseded claim.
	Succeed(ctx context.Context, e Entry, at time.Time) error
	// Fail reschedules the claimed entry e for retryAt, or settles it as failed once MaxAttempts is reached.
	Fail(ctx context.Context, e Entry, retryAt time.Time, cause string) error
	// Requeue resets the failed entry id/target to pending, attempt 0, for a fresh claim.
	Requeue(ctx context.Context, id uuid.UUID, target string) error
	// RequeueFailed resets every failed entry of target to pending, attempt 0, and returns how many it touched.
	RequeueFailed(ctx context.Context, target string) (int, error)
	// ByID returns the entry id of target, or *NotFoundError when it is absent from the caller's org.
	ByID(ctx context.Context, id uuid.UUID, target string) (Entry, error)
	// IDByPublicID returns the id of target's entry whose public id is publicID, or *NotFoundError when it is absent from the caller's org.
	IDByPublicID(ctx context.Context, publicID, target string) (uuid.UUID, error)
	// ListByTarget returns up to limit entries of target, newest first.
	ListByTarget(ctx context.Context, target string, limit int) ([]Entry, error)
}

// MaxCauseLen bounds the failure cause persisted on an entry.
const MaxCauseLen = 1024

func truncateCause(cause string) string {
	return apperror.TruncateCause(cause, MaxCauseLen)
}
