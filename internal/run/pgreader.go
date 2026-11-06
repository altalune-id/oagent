package run

import (
	"context"
	"fmt"
	"time"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/go-jet/jet/v2/qrm"
	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/tenant"
)

// OpenRun loads the thread's interrupted run, the one a claim resumes; nil when there is none.
func (s *postgresStore) OpenRun(ctx context.Context, threadID uuid.UUID) (*Run, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	r := s.runs
	stmt := postgres.SELECT(r.AllColumns).
		FROM(r).
		WHERE(r.ThreadID.EQ(postgres.UUID(threadID)).
			AND(r.OrgID.EQ(postgres.UUID(tc.OrgID))).
			AND(r.Status.EQ(postgres.String(string(StatusInterrupted))))).
		LIMIT(1)
	var row runRow
	if err := db.ReadPrimary(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &row) }); err != nil {
		if db.IsNoRows(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("run.postgres: OpenRun: %w", err)
	}
	return row.toRun(), nil
}

// StoredResults returns the run's checkpointed tool results, oldest first.
func (s *postgresStore) StoredResults(ctx context.Context, runID uuid.UUID) ([]StoredResult, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	m := s.messages
	stmt := postgres.SELECT(m.ToolCallID, m.Body).
		FROM(m).
		WHERE(m.RunID.EQ(postgres.UUID(runID)).
			AND(m.OrgID.EQ(postgres.UUID(tc.OrgID))).
			AND(m.ToolCallID.IS_NOT_NULL())).
		ORDER_BY(m.Seq.ASC())
	var rows []struct {
		ToolCallID string `alias:"messages.tool_call_id"`
		Body       string `alias:"messages.body"`
	}
	if err := db.ReadPrimary(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &rows) }); err != nil && !db.IsNoRows(err) {
		return nil, fmt.Errorf("run.postgres: StoredResults: %w", err)
	}
	out := make([]StoredResult, len(rows))
	for i, r := range rows {
		out[i] = StoredResult{ToolUseID: r.ToolCallID, Body: []byte(r.Body)}
	}
	return out, nil
}

// Expired returns the threads whose lease ended before now, and those with a running run and no lease at all.
func (s *postgresStore) Expired(ctx context.Context, now time.Time) ([]uuid.UUID, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	t, r := s.threads, s.runs
	orphan := postgres.EXISTS(postgres.SELECT(r.ID).FROM(r).
		WHERE(r.ThreadID.EQ(t.ID).AND(r.Status.EQ(postgres.String(string(StatusRunning))))))
	stmt := postgres.SELECT(t.ID).
		FROM(t).
		WHERE(t.OrgID.EQ(postgres.UUID(tc.OrgID)).
			AND(t.LeaseUntil.LT(postgres.TimestampzT(now.UTC())).OR(t.LeaseUntil.IS_NULL().AND(orphan)))).
		ORDER_BY(t.UpdatedAt.ASC())
	return s.threadIDs(ctx, "Expired", stmt)
}

// PendingUnleased returns up to limit threads with pending messages or an interrupted run, and no live lease at now.
func (s *postgresStore) PendingUnleased(ctx context.Context, now time.Time, limit int) ([]uuid.UUID, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	t, m := s.threads, s.messages
	r := s.runs
	pending := postgres.SELECT(m.ID).
		FROM(m).
		WHERE(m.ThreadID.EQ(t.ID).AND(m.Status.EQ(postgres.String(string(MessagePending)))))
	resumable := postgres.SELECT(r.ID).
		FROM(r).
		WHERE(r.ThreadID.EQ(t.ID).AND(r.Status.EQ(postgres.String(string(StatusInterrupted)))))
	stmt := postgres.SELECT(t.ID).
		FROM(t).
		WHERE(t.OrgID.EQ(postgres.UUID(tc.OrgID)).
			AND(t.LeaseUntil.IS_NULL().OR(t.LeaseUntil.LT(postgres.TimestampzT(now.UTC())))).
			AND(postgres.EXISTS(pending).OR(postgres.EXISTS(resumable)))).
		ORDER_BY(t.UpdatedAt.ASC()).
		LIMIT(int64(max(limit, 1)))
	return s.threadIDs(ctx, "PendingUnleased", stmt)
}

func (s *postgresStore) threadIDs(ctx context.Context, op string, stmt postgres.SelectStatement) ([]uuid.UUID, error) {
	var rows []struct {
		ID uuid.UUID `alias:"threads.id"`
	}
	if err := db.ReadPrimary(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &rows) }); err != nil && !db.IsNoRows(err) {
		return nil, fmt.Errorf("run.postgres: %s: %w", op, err)
	}
	out := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		out[i] = r.ID
	}
	return out, nil
}

// Thread loads a thread of the org on ctx.
func (s *postgresStore) Thread(ctx context.Context, threadID uuid.UUID) (*Thread, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	t := s.threads
	stmt := postgres.SELECT(t.ID, t.PublicID, t.OrgID, t.ProjectID, t.AgentID, t.Kind, t.ExternalKey, t.CancelRequested).FROM(t).
		WHERE(t.ID.EQ(postgres.UUID(threadID)).AND(t.OrgID.EQ(postgres.UUID(tc.OrgID))))
	var row struct {
		ID          uuid.UUID `alias:"threads.id"`
		PublicID    string    `alias:"threads.public_id"`
		OrgID       uuid.UUID `alias:"threads.org_id"`
		ProjectID   uuid.UUID `alias:"threads.project_id"`
		AgentID     uuid.UUID `alias:"threads.agent_id"`
		Kind        string    `alias:"threads.kind"`
		ExternalKey string    `alias:"threads.external_key"`
		Cancel      bool      `alias:"threads.cancel_requested"`
	}
	if err := db.ReadPrimary(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &row) }); err != nil {
		if db.IsNoRows(err) {
			return nil, &ThreadNotFoundError{ID: threadID.String()}
		}
		return nil, fmt.Errorf("run.postgres: Thread: %w", err)
	}
	return &Thread{
		ID: row.ID, PublicID: row.PublicID, OrgID: row.OrgID, ProjectID: row.ProjectID, AgentID: row.AgentID,
		Kind: row.Kind, ExternalKey: row.ExternalKey, CancelRequested: row.Cancel,
	}, nil
}

// History returns the thread's consumed messages in seq order, leaving out those of exceptRun.
func (s *postgresStore) History(ctx context.Context, threadID, exceptRun uuid.UUID) ([]Message, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	m := s.messages
	stmt := postgres.SELECT(m.AllColumns).FROM(m).
		WHERE(m.ThreadID.EQ(postgres.UUID(threadID)).
			AND(m.OrgID.EQ(postgres.UUID(tc.OrgID))).
			AND(m.Status.EQ(postgres.String(string(MessageConsumed)))).
			AND(m.RunID.IS_NULL().OR(m.RunID.NOT_EQ(postgres.UUID(exceptRun))))).
		ORDER_BY(m.Seq.ASC())
	var rows []messageRow
	if err := db.ReadPrimary(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &rows) }); err != nil && !db.IsNoRows(err) {
		return nil, fmt.Errorf("run.postgres: History: %w", err)
	}
	return toMessages(rows), nil
}

// HasPending reports whether the thread has a message no run has taken yet.
func (s *postgresStore) HasPending(ctx context.Context, threadID uuid.UUID) (bool, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return false, err
	}
	m := s.messages
	stmt := postgres.SELECT(m.ID).FROM(m).
		WHERE(m.ThreadID.EQ(postgres.UUID(threadID)).
			AND(m.OrgID.EQ(postgres.UUID(tc.OrgID))).
			AND(m.Status.EQ(postgres.String(string(MessagePending))))).
		LIMIT(1)
	var rows []struct {
		ID uuid.UUID `alias:"messages.id"`
	}
	if err := db.ReadPrimary(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &rows) }); err != nil && !db.IsNoRows(err) {
		return false, fmt.Errorf("run.postgres: HasPending: %w", err)
	}
	return len(rows) > 0, nil
}
