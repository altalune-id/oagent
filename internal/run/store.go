package run

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Store persists threads' inboxes, leases, runs, checkpoints and snapshots; every method reads the tenant on ctx.
type Store interface {
	Reader
	Writer
}

// Reader reads runs and the threads a sweep must act on.
type Reader interface {
	Thread(ctx context.Context, threadID uuid.UUID) (*Thread, error)
	History(ctx context.Context, threadID, exceptRun uuid.UUID) ([]Message, error)
	HasPending(ctx context.Context, threadID uuid.UUID) (bool, error)
	OpenRun(ctx context.Context, threadID uuid.UUID) (*Run, error)
	StoredResults(ctx context.Context, runID uuid.UUID) ([]StoredResult, error)
	Expired(ctx context.Context, now time.Time) ([]uuid.UUID, error)
	PendingUnleased(ctx context.Context, now time.Time, limit int) ([]uuid.UUID, error)
}

// Writer writes the inbox and, fenced by the lease token, everything a turn changes.
type Writer interface {
	AppendMessage(ctx context.Context, threadID uuid.UUID, m Message) (seq int64, err error)
	Claim(ctx context.Context, threadID uuid.UUID, token string, ttl time.Duration) (bool, error)
	Heartbeat(ctx context.Context, threadID uuid.UUID, token string, ttl time.Duration) (cancel bool, err error)
	Release(ctx context.Context, threadID uuid.UUID, token string) error
	StartRun(ctx context.Context, token string, r *Run) ([]Message, error)
	PutCheckpoint(ctx context.Context, token string, runID uuid.UUID, toolCallID string, msgs []Message) error
	Settle(ctx context.Context, token string, r *Run, u Usage, final []Message) error
	RequestCancel(ctx context.Context, threadID uuid.UUID) error
	Reclaim(ctx context.Context, threadID uuid.UUID) (bool, error)
}
