package run

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"time"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/go-jet/jet/v2/qrm"
	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/schema/entity"
)

type threadLock struct {
	OrgID     uuid.UUID `alias:"threads.org_id"`
	ProjectID uuid.UUID `alias:"threads.project_id"`
	NextSeq   int64     `alias:"threads.next_seq"`
}

func (s *postgresStore) lock(ctx context.Context, tx *sql.Tx, orgID, threadID uuid.UUID, token *string) (threadLock, error) {
	t := s.threads
	where := t.ID.EQ(postgres.UUID(threadID)).AND(t.OrgID.EQ(postgres.UUID(orgID)))
	if token != nil {
		where = where.AND(t.LeaseOwner.EQ(postgres.String(*token)))
	}
	stmt := postgres.SELECT(t.OrgID, t.ProjectID, t.NextSeq).FROM(t).WHERE(where).FOR(postgres.UPDATE())
	var row threadLock
	if err := stmt.QueryContext(ctx, tx, &row); err != nil {
		if !db.IsNoRows(err) {
			return row, fmt.Errorf("run.postgres: lock thread: %w", err)
		}
		if token != nil {
			return row, &LeaseLostError{ThreadID: threadID.String()}
		}
		return row, &ThreadNotFoundError{ID: threadID.String()}
	}
	return row, nil
}

func (s *postgresStore) insertMessages(
	ctx context.Context, tx *sql.Tx, threadID uuid.UUID, th threadLock, msgs []Message,
) ([]Message, error) {
	if len(msgs) == 0 {
		return nil, nil
	}
	m := s.messages
	stmt := m.INSERT(m.AllColumns)
	out := make([]Message, len(msgs))
	now := time.Now().UTC()
	for i, msg := range msgs {
		if msg.ID == uuid.Nil {
			msg.ID = uuid.Must(uuid.NewV7())
		}
		msg.OrgID, msg.ProjectID, msg.ThreadID, msg.Seq, msg.CreatedAt = th.OrgID, th.ProjectID, threadID, th.NextSeq+int64(i), now
		out[i] = msg
		stmt = stmt.VALUES(
			msg.ID, msg.OrgID, msg.ProjectID, msg.ThreadID, msg.Seq, string(msg.Role), jsonb(msg.Body),
			orNull(msg.SpeakerID, func(v uuid.UUID) postgres.Expression { return postgres.UUID(v) }),
			string(msg.Status), textOrNull(msg.ExternalMessageID),
			orNull(msg.RunID, func(v uuid.UUID) postgres.Expression { return postgres.UUID(v) }),
			textOrNull(msg.ToolCallID), now,
		)
	}
	if _, err := stmt.ExecContext(ctx, tx); err != nil {
		return nil, fmt.Errorf("run.postgres: insert messages: %w", err)
	}
	t := s.threads
	bump := t.UPDATE(t.NextSeq, t.UpdatedAt).
		SET(postgres.Int(th.NextSeq+int64(len(msgs))), postgres.NOW()).
		WHERE(t.ID.EQ(postgres.UUID(threadID)))
	if _, err := bump.ExecContext(ctx, tx); err != nil {
		return nil, fmt.Errorf("run.postgres: bump seq: %w", err)
	}
	return out, nil
}

// AppendMessage adds m to the thread's inbox as pending; a known external message id returns its seq and adds nothing.
func (s *postgresStore) AppendMessage(ctx context.Context, threadID uuid.UUID, msg Message) (int64, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return 0, err
	}
	var seq int64
	err = db.InTx(ctx, s.pool, func(tx *sql.Tx) error {
		th, err := s.lock(ctx, tx, tc.OrgID, threadID, nil)
		if err != nil {
			return err
		}
		if msg.ExternalMessageID != "" {
			m := s.messages
			known := postgres.SELECT(m.Seq).FROM(m).
				WHERE(m.ThreadID.EQ(postgres.UUID(threadID)).AND(m.ExternalMessageID.EQ(postgres.String(msg.ExternalMessageID))))
			var row struct {
				Seq int64 `alias:"messages.seq"`
			}
			err := known.QueryContext(ctx, tx, &row)
			if err == nil {
				seq = row.Seq
				return nil
			}
			if !db.IsNoRows(err) {
				return fmt.Errorf("run.postgres: AppendMessage: %w", err)
			}
		}
		msg.Status, msg.RunID = MessagePending, nil
		added, err := s.insertMessages(ctx, tx, threadID, th, []Message{msg})
		if err != nil {
			return err
		}
		seq = added[0].Seq
		return nil
	})
	return seq, err
}

// Claim leases the thread to token for ttl when no live lease holds it and its project is under its concurrency cap.
func (s *postgresStore) Claim(ctx context.Context, threadID uuid.UUID, token string, ttl time.Duration) (bool, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return false, err
	}
	won := false
	err = db.NewTransactor(s.pool).RunInTx(ctx, func(ctx context.Context) error {
		tx, _ := db.CurrentTx(ctx)
		t := s.threads
		owner := postgres.SELECT(t.ProjectID).FROM(t).WHERE(t.ID.EQ(postgres.UUID(threadID)).AND(t.OrgID.EQ(postgres.UUID(tc.OrgID))))
		var row struct {
			ProjectID uuid.UUID `alias:"threads.project_id"`
		}
		if err := owner.QueryContext(ctx, tx, &row); err != nil {
			if db.IsNoRows(err) {
				return nil
			}
			return fmt.Errorf("run.postgres: Claim: %w", err)
		}
		if err := db.LockInTx(ctx, "run-project:"+row.ProjectID.String()); err != nil {
			return err
		}
		live := postgres.SELECT(postgres.COUNT(t.ID).AS("live.count")).FROM(t).
			WHERE(t.ProjectID.EQ(postgres.UUID(row.ProjectID)).AND(t.LeaseUntil.GT(postgres.NOW())))
		var count struct {
			Count int `alias:"live.count"`
		}
		if err := live.QueryContext(ctx, tx, &count); err != nil {
			return fmt.Errorf("run.postgres: Claim: count leases: %w", err)
		}
		if count.Count >= s.maxPerProject {
			return nil
		}
		stmt := t.UPDATE(t.LeaseOwner, t.LeaseUntil, t.UpdatedAt).
			SET(postgres.String(token), postgres.NOW().ADD(postgres.INTERVALd(ttl)), postgres.NOW()).
			WHERE(t.ID.EQ(postgres.UUID(threadID)).
				AND(t.OrgID.EQ(postgres.UUID(tc.OrgID))).
				AND(t.LeaseUntil.IS_NULL().OR(t.LeaseUntil.LT(postgres.NOW()))))
		res, err := stmt.ExecContext(ctx, tx)
		if err != nil {
			return fmt.Errorf("run.postgres: Claim: %w", err)
		}
		n, err := res.RowsAffected()
		won = n == 1
		return err
	})
	return won && err == nil, err
}

// Heartbeat extends token's lease by ttl and reports whether a cancel was requested.
func (s *postgresStore) Heartbeat(ctx context.Context, threadID uuid.UUID, token string, ttl time.Duration) (bool, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return false, err
	}
	t := s.threads
	stmt := t.UPDATE(t.LeaseUntil).
		SET(postgres.NOW().ADD(postgres.INTERVALd(ttl))).
		WHERE(t.ID.EQ(postgres.UUID(threadID)).
			AND(t.OrgID.EQ(postgres.UUID(tc.OrgID))).
			AND(t.LeaseOwner.EQ(postgres.String(token)))).
		RETURNING(t.CancelRequested)
	var row struct {
		CancelRequested bool `alias:"threads.cancel_requested"`
	}
	err = db.Write(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &row) })
	if db.IsNoRows(err) {
		return false, &LeaseLostError{ThreadID: threadID.String()}
	}
	if err != nil {
		return false, fmt.Errorf("run.postgres: Heartbeat: %w", err)
	}
	return row.CancelRequested, nil
}

// Release ends token's lease.
func (s *postgresStore) Release(ctx context.Context, threadID uuid.UUID, token string) error {
	tc, err := tenant.From(ctx)
	if err != nil {
		return err
	}
	t := s.threads
	stmt := t.UPDATE(t.LeaseOwner, t.LeaseUntil).
		SET(entity.NullText(), entity.NullTimestampz()).
		WHERE(t.ID.EQ(postgres.UUID(threadID)).
			AND(t.OrgID.EQ(postgres.UUID(tc.OrgID))).
			AND(t.LeaseOwner.EQ(postgres.String(token))))
	n, err := s.exec(ctx, "Release", stmt)
	if err != nil {
		return err
	}
	if n == 0 {
		return &LeaseLostError{ThreadID: threadID.String()}
	}
	return nil
}

// RequestCancel flags the thread so its run stops: the turn holding the lease at its next heartbeat, an idle run at its next claim.
func (s *postgresStore) RequestCancel(ctx context.Context, threadID uuid.UUID) error {
	tc, err := tenant.From(ctx)
	if err != nil {
		return err
	}
	t := s.threads
	stmt := t.UPDATE(t.CancelRequested).
		SET(postgres.Bool(true)).
		WHERE(t.ID.EQ(postgres.UUID(threadID)).AND(t.OrgID.EQ(postgres.UUID(tc.OrgID))))
	n, err := s.exec(ctx, "RequestCancel", stmt)
	if err != nil {
		return err
	}
	if n == 0 {
		return &ThreadNotFoundError{ID: threadID.String()}
	}
	return nil
}

// Reclaim interrupts the thread's running run and clears its lease when no live lease holds it; false means there was nothing to reclaim.
func (s *postgresStore) Reclaim(ctx context.Context, threadID uuid.UUID) (bool, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return false, err
	}
	reclaimed := false
	err = db.InTx(ctx, s.pool, func(tx *sql.Tx) error {
		if _, err := s.lock(ctx, tx, tc.OrgID, threadID, nil); err != nil {
			if IsThreadNotFoundError(err) {
				return nil
			}
			return err
		}
		t, r := s.threads, s.runs
		unleased := postgres.EXISTS(postgres.SELECT(t.ID).FROM(t).
			WHERE(t.ID.EQ(postgres.UUID(threadID)).AND(t.LeaseUntil.IS_NULL().OR(t.LeaseUntil.LT(postgres.NOW())))))
		interrupt := r.UPDATE(r.Status, r.UpdatedAt).
			SET(postgres.String(string(StatusInterrupted)), postgres.NOW()).
			WHERE(r.ThreadID.EQ(postgres.UUID(threadID)).
				AND(r.OrgID.EQ(postgres.UUID(tc.OrgID))).
				AND(r.Status.EQ(postgres.String(string(StatusRunning)))).
				AND(unleased))
		interrupted, err := rowsOf(interrupt.ExecContext(ctx, tx))
		if err != nil {
			return err
		}
		expired := t.UPDATE(t.LeaseOwner, t.LeaseUntil, t.UpdatedAt).
			SET(entity.NullText(), entity.NullTimestampz(), postgres.NOW()).
			WHERE(t.ID.EQ(postgres.UUID(threadID)).AND(t.LeaseUntil.LT(postgres.NOW())))
		cleared, err := rowsOf(expired.ExecContext(ctx, tx))
		reclaimed = interrupted > 0 || cleared > 0
		return err
	})
	return reclaimed && err == nil, err
}

func rowsOf(res sql.Result, err error) (int64, error) {
	if err != nil {
		return 0, fmt.Errorf("run.postgres: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("run.postgres: %w", err)
	}
	return n, nil
}

func (s *postgresStore) clearCancel(ctx context.Context, tx *sql.Tx, threadID uuid.UUID) error {
	t := s.threads
	unset := t.UPDATE(t.CancelRequested).SET(postgres.Bool(false)).WHERE(t.ID.EQ(postgres.UUID(threadID)))
	_, err := rowsOf(unset.ExecContext(ctx, tx))
	return err
}

// StartRun stores r under token's lease: a new run takes every pending message in seq order, a resumed one returns its own.
func (s *postgresStore) StartRun(ctx context.Context, token string, r *Run) ([]Message, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	var taken []Message
	err = db.InTx(ctx, s.pool, func(tx *sql.Tx) error {
		if _, err := s.lock(ctx, tx, tc.OrgID, r.ThreadID, &token); err != nil {
			return err
		}
		if r.Attempt > 1 {
			return s.resume(ctx, tx, tc.OrgID, r, &taken)
		}
		if err := s.insertRun(ctx, tx, r); err != nil {
			return err
		}
		if err := s.clearCancel(ctx, tx, r.ThreadID); err != nil {
			return err
		}
		m := s.messages
		take := m.UPDATE(m.Status, m.RunID).
			SET(postgres.String(string(MessageConsumed)), postgres.UUID(r.ID)).
			WHERE(m.ThreadID.EQ(postgres.UUID(r.ThreadID)).
				AND(m.OrgID.EQ(postgres.UUID(tc.OrgID))).
				AND(m.Status.EQ(postgres.String(string(MessagePending))))).
			RETURNING(m.AllColumns)
		var rows []messageRow
		if err := take.QueryContext(ctx, tx, &rows); err != nil && !db.IsNoRows(err) {
			return fmt.Errorf("run.postgres: StartRun: take pending: %w", err)
		}
		if len(rows) == 0 {
			return ErrNothingPending
		}
		taken = toMessages(rows)
		slices.SortFunc(taken, func(a, b Message) int { return int(a.Seq - b.Seq) })
		return nil
	})
	if err != nil {
		return nil, err
	}
	return taken, nil
}

func (s *postgresStore) insertRun(ctx context.Context, tx *sql.Tx, r *Run) error {
	price := func(p func(*Prices) *int64) postgres.Expression {
		if r.Prices == nil {
			return postgres.NULL
		}
		return orNull(p(r.Prices), func(v int64) postgres.Expression { return postgres.Int(v) })
	}
	t := s.runs
	stmt := t.INSERT(
		t.ID, t.PublicID, t.OrgID, t.ProjectID, t.ThreadID, t.AgentVersionID, t.Status, t.IdempotencyKey, t.Attempt,
		t.PriceInputMicros, t.PriceOutputMicros, t.PriceCacheReadMicros, t.PriceCacheWriteMicros,
		t.StartedAt, t.CreatedAt, t.UpdatedAt,
	).VALUES(
		r.ID, r.PublicID, r.OrgID, r.ProjectID, r.ThreadID, r.AgentVersionID, string(r.Status), textOrNull(r.IdempotencyKey), r.Attempt,
		price(func(p *Prices) *int64 { return p.Input }), price(func(p *Prices) *int64 { return p.Output }),
		price(func(p *Prices) *int64 { return p.CacheRead }), price(func(p *Prices) *int64 { return p.CacheWrite }),
		r.StartedAt.UTC(), r.StartedAt.UTC(), r.UpdatedAt.UTC(),
	)
	if _, err := stmt.ExecContext(ctx, tx); err != nil {
		return fmt.Errorf("run.postgres: insert run: %w", err)
	}
	return nil
}

func (s *postgresStore) resume(ctx context.Context, tx *sql.Tx, orgID uuid.UUID, r *Run, taken *[]Message) error {
	t := s.runs
	stmt := t.UPDATE(t.Status, t.Attempt, t.UpdatedAt).
		SET(postgres.String(string(r.Status)), postgres.Int32(r.Attempt), postgres.TimestampzT(r.UpdatedAt.UTC())).
		WHERE(t.ID.EQ(postgres.UUID(r.ID)).
			AND(t.OrgID.EQ(postgres.UUID(orgID))).
			AND(t.Status.EQ(postgres.String(string(StatusInterrupted)))))
	res, err := stmt.ExecContext(ctx, tx)
	if err != nil {
		return fmt.Errorf("run.postgres: resume: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil || n != 1 {
		return ErrNothingPending
	}
	m := s.messages
	mine := postgres.SELECT(m.AllColumns).FROM(m).
		WHERE(m.RunID.EQ(postgres.UUID(r.ID)).AND(m.OrgID.EQ(postgres.UUID(orgID))).AND(m.ToolCallID.IS_NULL())).
		ORDER_BY(m.Seq.ASC())
	var rows []messageRow
	if err := mine.QueryContext(ctx, tx, &rows); err != nil && !db.IsNoRows(err) {
		return fmt.Errorf("run.postgres: resume: messages: %w", err)
	}
	*taken = toMessages(rows)
	return nil
}

// PutCheckpoint stores a tool call's messages on the run under token's lease; a tool call already stored is a no-op.
func (s *postgresStore) PutCheckpoint(ctx context.Context, token string, runID uuid.UUID, toolCallID string, msgs []Message) error {
	tc, err := tenant.From(ctx)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(msgs, func(m Message) bool { return m.ToolCallID == toolCallID }) {
		return fmt.Errorf("run.postgres: PutCheckpoint: no message carries tool call %q", toolCallID)
	}
	return db.InTx(ctx, s.pool, func(tx *sql.Tx) error {
		threadID, err := s.threadOf(ctx, tx, tc.OrgID, runID)
		if err != nil {
			return err
		}
		th, err := s.lock(ctx, tx, tc.OrgID, threadID, &token)
		if err != nil {
			return err
		}
		m := s.messages
		known := postgres.SELECT(m.ID).FROM(m).
			WHERE(m.RunID.EQ(postgres.UUID(runID)).AND(m.ToolCallID.EQ(postgres.String(toolCallID))))
		var row struct {
			ID uuid.UUID `alias:"messages.id"`
		}
		err = known.QueryContext(ctx, tx, &row)
		if err == nil {
			return nil
		}
		if !db.IsNoRows(err) {
			return fmt.Errorf("run.postgres: PutCheckpoint: %w", err)
		}
		_, err = s.insertMessages(ctx, tx, threadID, th, consumedBy(runID, msgs))
		return err
	})
}

func consumedBy(runID uuid.UUID, msgs []Message) []Message {
	out := slices.Clone(msgs)
	for i := range out {
		out[i].Status, out[i].RunID = MessageConsumed, &runID
	}
	return out
}

func (s *postgresStore) threadOf(ctx context.Context, tx *sql.Tx, orgID, runID uuid.UUID) (uuid.UUID, error) {
	t := s.runs
	stmt := postgres.SELECT(t.ThreadID).FROM(t).WHERE(t.ID.EQ(postgres.UUID(runID)).AND(t.OrgID.EQ(postgres.UUID(orgID))))
	var row struct {
		ThreadID uuid.UUID `alias:"runs.thread_id"`
	}
	if err := stmt.QueryContext(ctx, tx, &row); err != nil {
		if db.IsNoRows(err) {
			return uuid.Nil, &NotFoundError{ID: runID.String()}
		}
		return uuid.Nil, fmt.Errorf("run.postgres: run thread: %w", err)
	}
	return row.ThreadID, nil
}

// Settle commits r's end under token's lease, joining the transaction on ctx: status, usage u, cost and the final messages.
func (s *postgresStore) Settle(ctx context.Context, token string, r *Run, u Usage, final []Message) error {
	if !r.Status.Terminal() && r.Status != StatusInterrupted {
		return &UnsettledError{Status: r.Status}
	}
	tc, err := tenant.From(ctx)
	if err != nil {
		return err
	}
	return db.InTx(ctx, s.pool, func(tx *sql.Tx) error {
		th, err := s.lock(ctx, tx, tc.OrgID, r.ThreadID, &token)
		if err != nil {
			return err
		}
		if _, err := s.insertMessages(ctx, tx, r.ThreadID, th, consumedBy(r.ID, final)); err != nil {
			return err
		}
		if err := s.settleRun(ctx, tx, tc.OrgID, r, u); err != nil {
			return err
		}
		return s.clearCancel(ctx, tx, r.ThreadID)
	})
}

func (s *postgresStore) settleRun(ctx context.Context, tx *sql.Tx, orgID uuid.UUID, r *Run, u Usage) error {
	var cost = postgres.NULL
	if micros, priced := Cost(u, r.Prices); priced {
		cost = postgres.Int(micros)
	}
	var latency, ended = postgres.NULL, postgres.NULL
	if r.EndedAt != nil {
		latency = postgres.Int(min(r.EndedAt.Sub(r.StartedAt).Milliseconds(), int64(1<<31-1)))
		ended = postgres.TimestampzT(r.EndedAt.UTC())
	}
	t := s.runs
	stmt := t.UPDATE(
		t.Status, t.Attempt, t.InputTokens, t.OutputTokens, t.CacheReadTokens, t.CacheWriteTokens, t.UsageEstimated,
		t.CostMicros, t.LatencyMS, t.ErrorCode, t.EndedAt, t.UpdatedAt,
	).SET(
		postgres.String(string(r.Status)), postgres.Int32(r.Attempt), postgres.Int(u.InputTokens), postgres.Int(u.OutputTokens),
		postgres.Int(u.CacheReadTokens), postgres.Int(u.CacheWriteTokens), postgres.Bool(u.Estimated),
		cost, latency, textOrNull(r.ErrorCode), ended, postgres.TimestampzT(r.UpdatedAt.UTC()),
	).WHERE(t.ID.EQ(postgres.UUID(r.ID)).AND(t.OrgID.EQ(postgres.UUID(orgID))))
	res, err := stmt.ExecContext(ctx, tx)
	if err != nil {
		return fmt.Errorf("run.postgres: Settle: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil || n != 1 {
		return &NotFoundError{ID: r.ID.String()}
	}
	r.Usage = u
	return nil
}

func (s *postgresStore) exec(ctx context.Context, op string, stmt postgres.Statement) (int64, error) {
	var res sql.Result
	err := db.Write(ctx, s.pool, func(q qrm.DB) (err error) {
		res, err = stmt.ExecContext(ctx, q)
		return err
	})
	if err != nil {
		return 0, fmt.Errorf("run.postgres: %s: %w", op, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("run.postgres: %s: %w", op, err)
	}
	return n, nil
}
