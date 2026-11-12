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
	// PendingMessages lists speaker's pending messages in a thread after seq afterSeq, in channel order, at most limit.
	PendingMessages(ctx context.Context, threadID, speaker uuid.UUID, afterSeq int64, limit int) ([]Message, error)
	// HoldLease fails with a *LeaseLostError unless token still holds the thread's lease, taking the thread lock in ctx's unit of work.
	HoldLease(ctx context.Context, token string, threadID uuid.UUID) error
	// AnswerMessage consumes pending message msgID into runID, under the thread's lease token.
	AnswerMessage(ctx context.Context, token string, threadID, msgID, runID uuid.UUID) error
	ThreadByExternalKey(ctx context.Context, agentID uuid.UUID, externalKey string) (*Thread, error)
	TryThreadByExternalKey(ctx context.Context, agentID uuid.UUID, externalKey string) (*Thread, error)
	RunByPublicID(ctx context.Context, publicID string) (*Run, error)
	RunByID(ctx context.Context, id uuid.UUID) (*Run, error)
	ThreadMessages(ctx context.Context, threadID uuid.UUID, afterSeq int64, limit int) ([]Message, error)
	ResolveThread(ctx context.Context, publicID string) (Ref, error)
	ResolveRun(ctx context.Context, publicID string) (Ref, error)
	ListThreads(ctx context.Context, projectID uuid.UUID, filter ThreadFilter, after Cursor, limit int) ([]ThreadSummary, error)
	ThreadSummary(ctx context.Context, threadID uuid.UUID) (*ThreadSummary, error)
	ListRuns(ctx context.Context, threadID uuid.UUID, after Cursor, limit int) ([]Run, error)
	ListAgentRuns(ctx context.Context, agentID uuid.UUID, after Cursor, limit int) ([]Run, error)
	EndUserActivity(ctx context.Context, endUserIDs []uuid.UUID) (map[uuid.UUID]EndUserActivity, error)
	EndUserThreads(ctx context.Context, endUserID uuid.UUID) ([]ThreadSummary, error)
	RunByIdempotencyKey(ctx context.Context, threadID uuid.UUID, key string) (*Run, error)
	MessageByExternalID(ctx context.Context, threadID uuid.UUID, externalID string) (*Message, error)
	OpenRun(ctx context.Context, threadID uuid.UUID) (*Run, error)
	StoredResults(ctx context.Context, runID uuid.UUID) ([]StoredResult, error)
	Expired(ctx context.Context, now time.Time) ([]uuid.UUID, error)
	PendingUnleased(ctx context.Context, now time.Time, limit int) ([]uuid.UUID, error)
	ReplyTexts(ctx context.Context, connectionID uuid.UUID) (ReplyTexts, error)
}

// Writer writes the inbox and, fenced by the lease token, everything a turn changes.
type Writer interface {
	AppendMessage(ctx context.Context, threadID uuid.UUID, m Message) (seq int64, err error)
	AppendChannelMessage(ctx context.Context, in ChannelMessage) (ChannelAppend, error)
	Claim(ctx context.Context, threadID uuid.UUID, token string, ttl time.Duration) (ClaimResult, error)
	ClaimAndAppend(ctx context.Context, threadID uuid.UUID, token string, ttl time.Duration, m Message) (ClaimResult, int64, error)
	UpsertAPIThread(ctx context.Context, projectID, agentID uuid.UUID, externalKey string, actor uuid.UUID) (*Thread, error)
	// UpsertTryThread is UpsertAPIThread for a console Try it thread, kept apart from API threads with the same key.
	UpsertTryThread(ctx context.Context, projectID, agentID uuid.UUID, externalKey string, actor uuid.UUID) (*Thread, error)
	Heartbeat(ctx context.Context, threadID uuid.UUID, token string, ttl time.Duration) (cancel bool, err error)
	Release(ctx context.Context, threadID uuid.UUID, token string) error
	StartRun(ctx context.Context, token string, r *Run) ([]Message, error)
	// ResumeApproval moves an awaiting run back to running under token's lease, with the same attempt; it returns the run's messages.
	ResumeApproval(ctx context.Context, token string, r *Run) ([]Message, error)
	PutCheckpoint(ctx context.Context, token string, runID uuid.UUID, toolCallID string, msgs []Message) error
	Settle(ctx context.Context, token string, r *Run, u Usage, final []Message) error
	RequestCancel(ctx context.Context, threadID, runID uuid.UUID) error
	Reclaim(ctx context.Context, threadID uuid.UUID) (bool, error)
}
