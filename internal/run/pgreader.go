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
	return s.oneThread(ctx, threadID.String(), t.ID.EQ(postgres.UUID(threadID)).AND(t.OrgID.EQ(postgres.UUID(tc.OrgID))))
}

// ThreadByExternalKey loads the thread of agentID that its channel calls externalKey.
func (s *postgresStore) ThreadByExternalKey(ctx context.Context, agentID uuid.UUID, externalKey string) (*Thread, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	var th *Thread
	err = db.Read(ctx, s.pool, func(q qrm.DB) error {
		th, err = s.threadByExternalKey(ctx, q, tc.OrgID, agentID, externalKey)
		return err
	})
	return th, err
}

type threadRow struct {
	ID          uuid.UUID  `alias:"threads.id"`
	PublicID    string     `alias:"threads.public_id"`
	OrgID       uuid.UUID  `alias:"threads.org_id"`
	ProjectID   uuid.UUID  `alias:"threads.project_id"`
	AgentID     uuid.UUID  `alias:"threads.agent_id"`
	Kind        string     `alias:"threads.kind"`
	ExternalKey string     `alias:"threads.external_key"`
	Cancel      bool       `alias:"threads.cancel_requested"`
	Actor       *uuid.UUID `alias:"threads.actor_end_user_id"`
}

func (r *threadRow) toThread() *Thread {
	th := &Thread{
		ID: r.ID, PublicID: r.PublicID, OrgID: r.OrgID, ProjectID: r.ProjectID, AgentID: r.AgentID,
		Kind: r.Kind, ExternalKey: r.ExternalKey, CancelRequested: r.Cancel,
	}
	if r.Actor != nil {
		th.ActorEndUserID = *r.Actor
	}
	return th
}

func (s *postgresStore) threadColumns() postgres.ProjectionList {
	t := s.threads
	return postgres.ProjectionList{
		t.ID, t.PublicID, t.OrgID, t.ProjectID, t.AgentID, t.Kind, t.ExternalKey, t.CancelRequested, t.ActorEndUserID,
	}
}

func (s *postgresStore) oneThread(ctx context.Context, name string, where postgres.BoolExpression) (*Thread, error) {
	stmt := postgres.SELECT(s.threadColumns()).FROM(s.threads).WHERE(where).LIMIT(1)
	var row threadRow
	if err := db.ReadPrimary(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &row) }); err != nil {
		if db.IsNoRows(err) {
			return nil, &ThreadNotFoundError{ID: name}
		}
		return nil, fmt.Errorf("run.postgres: Thread: %w", err)
	}
	return row.toThread(), nil
}

func (s *postgresStore) threadByExternalKey(
	ctx context.Context, q qrm.Queryable, orgID, agentID uuid.UUID, externalKey string,
) (*Thread, error) {
	t := s.threads
	stmt := postgres.SELECT(s.threadColumns()).FROM(t).
		WHERE(t.AgentID.EQ(postgres.UUID(agentID)).
			AND(t.ExternalKey.EQ(postgres.String(externalKey))).
			AND(t.OrgID.EQ(postgres.UUID(orgID)))).
		LIMIT(1)
	var row threadRow
	if err := stmt.QueryContext(ctx, q, &row); err != nil {
		if db.IsNoRows(err) {
			return nil, &ThreadNotFoundError{ID: externalKey}
		}
		return nil, fmt.Errorf("run.postgres: thread by external key: %w", err)
	}
	return row.toThread(), nil
}

// RunByPublicID loads a run of the org on ctx by its public id.
func (s *postgresStore) RunByPublicID(ctx context.Context, publicID string) (*Run, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	r := s.runs
	return s.oneRun(ctx, r.PublicID.EQ(postgres.String(publicID)).AND(r.OrgID.EQ(postgres.UUID(tc.OrgID))), &NotFoundError{ID: publicID})
}

// RunByIdempotencyKey loads the thread's run started for key, or nil when there is none.
func (s *postgresStore) RunByIdempotencyKey(ctx context.Context, threadID uuid.UUID, key string) (*Run, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	r := s.runs
	found, err := s.oneRun(ctx, r.ThreadID.EQ(postgres.UUID(threadID)).
		AND(r.IdempotencyKey.EQ(postgres.String(key))).
		AND(r.OrgID.EQ(postgres.UUID(tc.OrgID))), nil)
	if IsNotFoundError(err) {
		return nil, nil
	}
	return found, err
}

func (s *postgresStore) oneRun(ctx context.Context, where postgres.BoolExpression, missing error) (*Run, error) {
	stmt := postgres.SELECT(s.runs.AllColumns).FROM(s.runs).WHERE(where).LIMIT(1)
	var row runRow
	if err := db.ReadPrimary(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &row) }); err != nil {
		if db.IsNoRows(err) {
			if missing == nil {
				missing = &NotFoundError{}
			}
			return nil, missing
		}
		return nil, fmt.Errorf("run.postgres: run: %w", err)
	}
	return row.toRun(), nil
}

// MessageByExternalID loads the thread's message with externalID, or nil when there is none.
func (s *postgresStore) MessageByExternalID(ctx context.Context, threadID uuid.UUID, externalID string) (*Message, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	var m *Message
	err = db.ReadPrimary(ctx, s.pool, func(q qrm.DB) error {
		m, err = s.messageByExternalID(ctx, q, tc.OrgID, threadID, externalID)
		return err
	})
	return m, err
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

// RunByID loads a run of the org on ctx.
func (s *postgresStore) RunByID(ctx context.Context, id uuid.UUID) (*Run, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	r := s.runs
	return s.oneRun(ctx, r.ID.EQ(postgres.UUID(id)).AND(r.OrgID.EQ(postgres.UUID(tc.OrgID))), &NotFoundError{ID: id.String()})
}

// ThreadMessages returns up to limit of the thread's messages after afterSeq, oldest first, pending ones included.
func (s *postgresStore) ThreadMessages(ctx context.Context, threadID uuid.UUID, afterSeq int64, limit int) ([]Message, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	m := s.messages
	stmt := postgres.SELECT(m.AllColumns).FROM(m).
		WHERE(m.ThreadID.EQ(postgres.UUID(threadID)).
			AND(m.OrgID.EQ(postgres.UUID(tc.OrgID))).
			AND(m.Seq.GT(postgres.Int(afterSeq)))).
		ORDER_BY(m.Seq.ASC()).
		LIMIT(int64(max(limit, 1)))
	var rows []messageRow
	if err := db.Read(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &rows) }); err != nil && !db.IsNoRows(err) {
		return nil, fmt.Errorf("run.postgres: ThreadMessages: %w", err)
	}
	return toMessages(rows), nil
}

// ResolveThread finds a thread by public id across tenants; every later read is scoped to the org it returns.
func (s *postgresStore) ResolveThread(ctx context.Context, publicID string) (Ref, error) {
	return s.resolve(ctx, s.resolveThread, publicID, &ThreadNotFoundError{ID: publicID})
}

// ResolveRun finds a run by public id across tenants; every later read is scoped to the org it returns.
func (s *postgresStore) ResolveRun(ctx context.Context, publicID string) (Ref, error) {
	return s.resolve(ctx, s.resolveRun, publicID, &NotFoundError{ID: publicID})
}

func (s *postgresStore) resolve(ctx context.Context, query, publicID string, missing error) (Ref, error) {
	var row struct {
		ID        uuid.UUID `alias:"ref.id"`
		OrgID     uuid.UUID `alias:"ref.org_id"`
		ProjectID uuid.UUID `alias:"ref.project_id"`
		AgentID   uuid.UUID `alias:"ref.agent_id"`
	}
	stmt := postgres.RawStatement(query, postgres.RawArgs{"#publicID": publicID})
	if err := db.Read(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &row) }); err != nil {
		if db.IsNoRows(err) {
			return Ref{}, missing
		}
		return Ref{}, fmt.Errorf("run.postgres: resolve: %w", err)
	}
	return Ref(row), nil
}

type summaryRow struct {
	Thread    threadRow `alias:"threads.*"`
	UpdatedAt time.Time `alias:"threads.updated_at"`
	OpenRun   *string   `alias:"open_run.status"`
}

func (r *summaryRow) toSummary() ThreadSummary {
	out := ThreadSummary{Thread: *r.Thread.toThread(), LastActivityAt: r.UpdatedAt}
	if r.OpenRun != nil {
		out.OpenRun = Status(*r.OpenRun)
	}
	return out
}

func (s *postgresStore) summaries(ctx context.Context, where postgres.BoolExpression, limit int64) ([]summaryRow, error) {
	t, r := s.threads, s.runs
	stmt := postgres.SELECT(s.threadColumns(), t.UpdatedAt, r.Status.AS("open_run.status")).
		FROM(t.LEFT_JOIN(r, r.ThreadID.EQ(t.ID).
			AND(r.Status.IN(postgres.String(string(StatusRunning)), postgres.String(string(StatusAwaitingApproval)))))).
		WHERE(where).
		ORDER_BY(t.UpdatedAt.DESC(), t.PublicID.DESC()).
		LIMIT(limit)
	var rows []summaryRow
	if err := db.Read(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &rows) }); err != nil && !db.IsNoRows(err) {
		return nil, fmt.Errorf("run.postgres: thread summaries: %w", err)
	}
	return rows, nil
}

// ListThreads lists a project's threads, newest activity first, optionally of one agent only.
func (s *postgresStore) ListThreads(ctx context.Context, projectID, agentID uuid.UUID, after Cursor, limit int) ([]ThreadSummary, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	t := s.threads
	where := t.OrgID.EQ(postgres.UUID(tc.OrgID)).AND(t.ProjectID.EQ(postgres.UUID(projectID)))
	if agentID != uuid.Nil {
		where = where.AND(t.AgentID.EQ(postgres.UUID(agentID)))
	}
	if !after.At.IsZero() {
		where = where.AND(postgres.ROW(t.UpdatedAt, t.PublicID).LT(postgres.ROW(postgres.TimestampzT(after.At), postgres.String(after.PublicID))))
	}
	rows, err := s.summaries(ctx, where, int64(max(limit, 1)))
	if err != nil {
		return nil, err
	}
	out := make([]ThreadSummary, len(rows))
	for i := range rows {
		out[i] = rows[i].toSummary()
	}
	return out, nil
}

// ThreadSummary loads one thread's summary with its message and run counts.
func (s *postgresStore) ThreadSummary(ctx context.Context, threadID uuid.UUID) (*ThreadSummary, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	t := s.threads
	rows, err := s.summaries(ctx, t.ID.EQ(postgres.UUID(threadID)).AND(t.OrgID.EQ(postgres.UUID(tc.OrgID))), 1)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, &ThreadNotFoundError{ID: threadID.String()}
	}
	out := rows[0].toSummary()
	m, r := s.messages, s.runs
	var counts struct {
		Messages int64 `alias:"counts.messages"`
		Runs     int64 `alias:"counts.runs"`
	}
	stmt := postgres.SELECT(
		postgres.SELECT(postgres.COUNT(postgres.STAR)).FROM(m).WHERE(m.ThreadID.EQ(postgres.UUID(threadID))).AS("counts.messages"),
		postgres.SELECT(postgres.COUNT(postgres.STAR)).FROM(r).WHERE(r.ThreadID.EQ(postgres.UUID(threadID))).AS("counts.runs"),
	)
	if err := db.Read(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &counts) }); err != nil {
		return nil, fmt.Errorf("run.postgres: thread counts: %w", err)
	}
	out.Messages, out.Runs = counts.Messages, counts.Runs
	return &out, nil
}

// ListRuns lists a thread's runs, newest first.
func (s *postgresStore) ListRuns(ctx context.Context, threadID uuid.UUID, after Cursor, limit int) ([]Run, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	r := s.runs
	where := r.ThreadID.EQ(postgres.UUID(threadID)).AND(r.OrgID.EQ(postgres.UUID(tc.OrgID)))
	if !after.At.IsZero() {
		where = where.AND(postgres.ROW(r.StartedAt, r.PublicID).LT(postgres.ROW(postgres.TimestampzT(after.At), postgres.String(after.PublicID))))
	}
	stmt := postgres.SELECT(r.AllColumns).FROM(r).WHERE(where).ORDER_BY(r.StartedAt.DESC(), r.PublicID.DESC()).LIMIT(int64(max(limit, 1)))
	var rows []runRow
	if err := db.Read(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &rows) }); err != nil && !db.IsNoRows(err) {
		return nil, fmt.Errorf("run.postgres: ListRuns: %w", err)
	}
	out := make([]Run, len(rows))
	for i := range rows {
		out[i] = *rows[i].toRun()
	}
	return out, nil
}
