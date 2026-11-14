package run

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/go-jet/jet/v2/qrm"
	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/nanoid"
	"altalune.id/oagent/schema/entity"
)

type threadLock struct {
	OrgID        uuid.UUID  `alias:"threads.org_id"`
	ProjectID    uuid.UUID  `alias:"threads.project_id"`
	NextSeq      int64      `alias:"threads.next_seq"`
	ConnectionID *uuid.UUID `alias:"threads.connection_id"`
}

func (s *postgresStore) lock(ctx context.Context, tx *sql.Tx, orgID, threadID uuid.UUID, token *string) (threadLock, error) {
	t := s.threads
	where := t.ID.EQ(postgres.UUID(threadID)).AND(t.OrgID.EQ(postgres.UUID(orgID)))
	if token != nil {
		where = where.AND(t.LeaseOwner.EQ(postgres.String(*token)))
	}
	stmt := postgres.SELECT(t.OrgID, t.ProjectID, t.NextSeq, t.ConnectionID).FROM(t).WHERE(where).FOR(postgres.UPDATE())
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
	stmt := m.INSERT(m.ID, m.OrgID, m.ProjectID, m.ThreadID, m.Seq, m.Role, m.Body, m.SpeakerID, m.Status, m.ExternalMessageID, m.RunID,
		m.ToolCallID, m.CreatedAt, m.RequestHash, m.ChannelTime, m.SkipReason)
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
			textOrNull(msg.ToolCallID), now, bytesOrNull(msg.RequestHash),
			orNull(msg.ChannelTime, func(v time.Time) postgres.Expression { return postgres.TimestampzT(v.UTC()) }),
			textOrNull(msg.SkipReason),
		)
	}
	// NOTE: a tool message is stored once per call: a later leg or replay repeats it, and messages_run_tool_call_id_key skips the repeat.
	conflict := stmt.ON_CONFLICT(m.RunID, m.ToolCallID).WHERE(m.ToolCallID.IS_NOT_NULL()).DO_NOTHING()
	if _, err := conflict.ExecContext(ctx, tx); err != nil {
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

// AppendChannelMessage upserts the connection's thread for the chat and stores the message, pending or skipped past the sender cap.
func (s *postgresStore) AppendChannelMessage(ctx context.Context, in ChannelMessage) (ChannelAppend, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return ChannelAppend{}, err
	}
	var out ChannelAppend
	err = db.InTx(ctx, s.pool, func(tx *sql.Tx) error {
		id, err := s.upsertChannelThread(ctx, tx, tc.OrgID, in)
		if err != nil {
			return err
		}
		out.ThreadID = id
		th, err := s.lock(ctx, tx, tc.OrgID, id, nil)
		if err != nil {
			return err
		}
		msg := in.Message
		if out.Duplicate, err = s.knownExternal(ctx, tx, id, msg.ExternalMessageID); err != nil || out.Duplicate {
			return err
		}
		recent, err := s.recentFromSpeaker(ctx, tx, id, msg.SpeakerID)
		if err != nil {
			return err
		}
		msg.Role, msg.Status, msg.RunID, msg.SkipReason = RoleUser, MessagePending, nil, ""
		if recent >= in.SenderCap {
			msg.Status, msg.SkipReason = MessageConsumed, SkipSenderRate
		}
		out.Pending = msg.Status == MessagePending
		_, err = s.insertMessages(ctx, tx, id, th, []Message{msg})
		return err
	})
	if err != nil {
		return ChannelAppend{}, fmt.Errorf("run.postgres: AppendChannelMessage: %w", err)
	}
	return out, nil
}

func (s *postgresStore) upsertChannelThread(ctx context.Context, tx *sql.Tx, orgID uuid.UUID, in ChannelMessage) (uuid.UUID, error) {
	t := s.threads
	now := time.Now().UTC()
	actor := postgres.NULL
	if in.Actor != uuid.Nil {
		actor = postgres.UUID(in.Actor)
	}
	stmt := t.INSERT(t.ID, t.PublicID, t.OrgID, t.ProjectID, t.AgentID, t.Kind, t.ExternalKey, t.ConnectionID, t.ChannelAddress, t.ReplyVia,
		t.ActorEndUserID, t.CreatedAt, t.UpdatedAt).
		VALUES(uuid.Must(uuid.NewV7()), nanoid.MustNew(21), orgID, in.ProjectID, in.AgentID, in.Kind, in.ChatKey, in.ConnectionID,
			textOrNull(in.ChannelAddress), textOrNull(in.ReplyVia), actor, now, now).
		ON_CONFLICT(t.ConnectionID, t.ExternalKey).WHERE(postgres.RawBool("connection_id IS NOT NULL")).
		// SECURITY: the conflict clause is guarded by org, so a key naming another tenant's thread never rewrites it.
		DO_UPDATE(postgres.SET(
			t.ChannelAddress.SET(postgres.StringExp(textOrNull(in.ChannelAddress))),
			t.ReplyVia.SET(postgres.StringExp(textOrNull(in.ReplyVia))),
		).WHERE(t.OrgID.EQ(postgres.UUID(orgID)))).
		RETURNING(t.ID)
	var row struct {
		ID uuid.UUID `alias:"threads.id"`
	}
	if err := stmt.QueryContext(ctx, tx, &row); err != nil {
		if db.IsNoRows(err) || db.IsRowSecurityViolation(err) {
			return uuid.Nil, &ThreadNotFoundError{ID: in.ChatKey}
		}
		return uuid.Nil, fmt.Errorf("upsert thread: %w", err)
	}
	return row.ID, nil
}

func (s *postgresStore) knownExternal(ctx context.Context, tx *sql.Tx, threadID uuid.UUID, externalID string) (bool, error) {
	if externalID == "" {
		return false, nil
	}
	m := s.messages
	stmt := postgres.SELECT(m.Seq).FROM(m).
		WHERE(m.ThreadID.EQ(postgres.UUID(threadID)).AND(m.ExternalMessageID.EQ(postgres.String(externalID))))
	var row struct {
		Seq int64 `alias:"messages.seq"`
	}
	err := stmt.QueryContext(ctx, tx, &row)
	if db.IsNoRows(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("known message: %w", err)
	}
	return true, nil
}

func (s *postgresStore) recentFromSpeaker(ctx context.Context, tx *sql.Tx, threadID uuid.UUID, speaker *uuid.UUID) (int, error) {
	if speaker == nil {
		return 0, nil
	}
	m := s.messages
	stmt := postgres.SELECT(postgres.COUNT(postgres.STAR).AS("count.n")).FROM(m).
		WHERE(m.ThreadID.EQ(postgres.UUID(threadID)).
			AND(m.SpeakerID.EQ(postgres.UUID(*speaker))).
			AND(m.CreatedAt.GT(postgres.NOW().SUB(postgres.INTERVALd(senderWindow)))))
	var row struct {
		N int `alias:"count.n"`
	}
	if err := stmt.QueryContext(ctx, tx, &row); err != nil {
		return 0, fmt.Errorf("count sender: %w", err)
	}
	return row.N, nil
}

// Claim leases the thread to token for ttl when it is free and its project is under the cap; the result says why not.
func (s *postgresStore) Claim(ctx context.Context, threadID uuid.UUID, token string, ttl time.Duration) (ClaimResult, error) {
	result := LeaseLive
	err := db.NewTransactor(s.pool).RunInTx(ctx, func(ctx context.Context) error {
		var err error
		result, err = s.claim(ctx, threadID, token, ttl)
		return err
	})
	if err != nil {
		return LeaseLive, err
	}
	return result, nil
}

func (s *postgresStore) claim(ctx context.Context, threadID uuid.UUID, token string, ttl time.Duration) (ClaimResult, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return LeaseLive, err
	}
	tx, _ := db.CurrentTx(ctx)
	t := s.threads
	head := postgres.SELECT(t.ProjectID, t.LeaseUntil.GT(postgres.NOW()).AS("thread.live")).FROM(t).
		WHERE(t.ID.EQ(postgres.UUID(threadID)).AND(t.OrgID.EQ(postgres.UUID(tc.OrgID)))).
		FOR(postgres.UPDATE())
	var row struct {
		ProjectID uuid.UUID `alias:"threads.project_id"`
		Live      *bool     `alias:"thread.live"`
	}
	if err := head.QueryContext(ctx, tx, &row); err != nil {
		if db.IsNoRows(err) {
			return LeaseLive, nil
		}
		return LeaseLive, fmt.Errorf("run.postgres: Claim: %w", err)
	}
	if row.Live != nil && *row.Live {
		return LeaseLive, nil
	}
	if err := db.LockInTx(ctx, "run-project:"+row.ProjectID.String()); err != nil {
		return LeaseLive, err
	}
	live := postgres.SELECT(postgres.COUNT(t.ID).AS("live.count")).FROM(t).
		WHERE(t.ProjectID.EQ(postgres.UUID(row.ProjectID)).AND(t.LeaseUntil.GT(postgres.NOW())))
	var count struct {
		Count int `alias:"live.count"`
	}
	if err := live.QueryContext(ctx, tx, &count); err != nil {
		return LeaseLive, fmt.Errorf("run.postgres: Claim: count leases: %w", err)
	}
	if count.Count >= s.maxPerProject {
		return ProjectBusy, nil
	}
	stmt := t.UPDATE(t.LeaseOwner, t.LeaseUntil, t.UpdatedAt).
		SET(postgres.String(token), postgres.NOW().ADD(postgres.INTERVALd(ttl)), postgres.NOW()).
		WHERE(t.ID.EQ(postgres.UUID(threadID)).AND(t.OrgID.EQ(postgres.UUID(tc.OrgID))))
	if _, err := rowsOf(stmt.ExecContext(ctx, tx)); err != nil {
		return LeaseLive, err
	}
	return Claimed, nil
}

// ClaimAndAppend claims the thread and adds m as pending in one transaction; anything but Claimed stores nothing.
func (s *postgresStore) ClaimAndAppend(
	ctx context.Context, threadID uuid.UUID, token string, ttl time.Duration, m Message,
) (ClaimResult, int64, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return LeaseLive, 0, err
	}
	result, seq := LeaseLive, int64(0)
	err = db.NewTransactor(s.pool).RunInTx(ctx, func(ctx context.Context) error {
		if result, err = s.claim(ctx, threadID, token, ttl); err != nil || result != Claimed {
			return err
		}
		tx, _ := db.CurrentTx(ctx)
		th, err := s.lock(ctx, tx, tc.OrgID, threadID, nil)
		if err != nil {
			return err
		}
		if m.ExternalMessageID != "" {
			known, err := s.messageByExternalID(ctx, tx, tc.OrgID, threadID, m.ExternalMessageID)
			if err != nil {
				return err
			}
			if known != nil {
				return &DuplicateMessageError{Seq: known.Seq}
			}
		}
		m.Status, m.RunID = MessagePending, nil
		added, err := s.insertMessages(ctx, tx, threadID, th, []Message{m})
		if err != nil {
			if db.IsUniqueViolation(err) {
				return &DuplicateMessageError{}
			}
			return err
		}
		seq = added[0].Seq
		return nil
	})
	if err != nil {
		return LeaseLive, 0, err
	}
	return result, seq, nil
}

func (s *postgresStore) messageByExternalID(
	ctx context.Context, q qrm.Queryable, orgID, threadID uuid.UUID, externalID string,
) (*Message, error) {
	m := s.messages
	stmt := postgres.SELECT(m.AllColumns).FROM(m).
		WHERE(m.ThreadID.EQ(postgres.UUID(threadID)).
			AND(m.OrgID.EQ(postgres.UUID(orgID))).
			AND(m.ExternalMessageID.EQ(postgres.String(externalID)))).
		LIMIT(1)
	var row messageRow
	if err := stmt.QueryContext(ctx, q, &row); err != nil {
		if db.IsNoRows(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("run.postgres: message by external id: %w", err)
	}
	msg := row.toMessage()
	return &msg, nil
}

// UpsertAPIThread returns agentID's API thread keyed externalKey, created bound to actor; another actor is a *ThreadActorMismatchError.
func (s *postgresStore) UpsertAPIThread(
	ctx context.Context, projectID, agentID uuid.UUID, externalKey string, actor uuid.UUID,
) (*Thread, error) {
	return s.upsertBoundThread(ctx, threadKindAPI, projectID, agentID, externalKey, actor)
}

// UpsertTryThread returns agentID's Try it thread keyed externalKey, created bound to actor; another actor is a *ThreadActorMismatchError.
func (s *postgresStore) UpsertTryThread(
	ctx context.Context, projectID, agentID uuid.UUID, externalKey string, actor uuid.UUID,
) (*Thread, error) {
	return s.upsertBoundThread(ctx, threadKindTry, projectID, agentID, externalKey, actor)
}

func (s *postgresStore) upsertBoundThread(
	ctx context.Context, kind string, projectID, agentID uuid.UUID, externalKey string, actor uuid.UUID,
) (*Thread, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	if actor == uuid.Nil {
		return nil, errNoActor
	}
	var th *Thread
	err = db.NewTransactor(s.pool).RunInTx(ctx, func(ctx context.Context) error {
		tx, _ := db.CurrentTx(ctx)
		t := s.threads
		now := time.Now().UTC()
		insert := t.INSERT(t.ID, t.PublicID, t.OrgID, t.ProjectID, t.AgentID, t.Kind, t.ExternalKey, t.ActorEndUserID, t.CreatedAt, t.UpdatedAt).
			VALUES(uuid.Must(uuid.NewV7()), nanoid.MustNew(21), tc.OrgID, projectID, agentID, kind, externalKey, actor, now, now).
			ON_CONFLICT(t.AgentID, t.ExternalKey).WHERE(t.Kind.EQ(postgres.String(kind))).DO_NOTHING()
		if _, err := insert.ExecContext(ctx, tx); err != nil {
			return fmt.Errorf("run.postgres: upsert %s thread: %w", kind, err)
		}
		th, err = s.threadByExternalKey(ctx, tx, kind, tc.OrgID, agentID, externalKey)
		return err
	})
	if err != nil {
		return nil, err
	}
	// SECURITY: an orphaned thread (its actor deleted) is bound to nobody, so it answers not found to everyone.
	if th.ActorEndUserID == uuid.Nil || th.ActorEndUserID != actor || th.Kind != kind {
		return nil, &ThreadActorMismatchError{ThreadID: th.ID.String()}
	}
	return th, nil
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

// RequestCancel flags the thread so runID stops, at the next heartbeat or the next claim; a run that already ended flags nothing.
func (s *postgresStore) RequestCancel(ctx context.Context, threadID, runID uuid.UUID) error {
	tc, err := tenant.From(ctx)
	if err != nil {
		return err
	}
	t, r := s.threads, s.runs
	open := postgres.SELECT(r.ID).FROM(r).WHERE(r.ID.EQ(postgres.UUID(runID)).
		AND(r.ThreadID.EQ(postgres.UUID(threadID))).
		AND(r.Status.IN(
			postgres.String(string(StatusRunning)), postgres.String(string(StatusAwaitingApproval)), postgres.String(string(StatusInterrupted)),
		)))
	stmt := t.UPDATE(t.CancelRequested).
		SET(postgres.Bool(true)).
		WHERE(t.ID.EQ(postgres.UUID(threadID)).AND(t.OrgID.EQ(postgres.UUID(tc.OrgID))).AND(postgres.EXISTS(open)))
	n, err := s.exec(ctx, "RequestCancel", stmt)
	if err != nil || n > 0 {
		return err
	}
	if _, err := s.Thread(ctx, threadID); err != nil {
		return err
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
	nothing := false
	err = db.InTx(ctx, s.pool, func(tx *sql.Tx) error {
		th, err := s.lock(ctx, tx, tc.OrgID, r.ThreadID, &token)
		if err != nil {
			return err
		}
		if r.Attempt > 1 {
			return s.resume(ctx, tx, tc.OrgID, r, &taken)
		}
		channel := th.ConnectionID != nil
		if channel {
			if err := s.skipStale(ctx, tx, tc.OrgID, r.ThreadID); err != nil {
				return err
			}
		}
		ids, err := s.toTake(ctx, tx, tc.OrgID, r.ThreadID, channel)
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			nothing = true
			return nil
		}
		if err := s.insertRun(ctx, tx, r); err != nil {
			return err
		}
		if err := s.clearCancel(ctx, tx, r.ThreadID); err != nil {
			return err
		}
		if taken, err = s.takeIDs(ctx, tx, r.ID, ids); err != nil {
			return err
		}
		return s.recordIdempotencyKey(ctx, tx, r, taken)
	})
	if err != nil {
		return nil, err
	}
	if nothing {
		return nil, ErrNothingPending
	}
	return taken, nil
}

func (s *postgresStore) skipStale(ctx context.Context, tx *sql.Tx, orgID, threadID uuid.UUID) error {
	m := s.messages
	stmt := m.UPDATE(m.Status, m.SkipReason).
		SET(postgres.String(string(MessageConsumed)), postgres.String(SkipStale)).
		WHERE(m.ThreadID.EQ(postgres.UUID(threadID)).
			AND(m.OrgID.EQ(postgres.UUID(orgID))).
			AND(m.Status.EQ(postgres.String(string(MessagePending)))).
			AND(m.ChannelTime.LT(postgres.TimestampzT(time.Now().UTC().Add(-s.maxAge)))))
	if _, err := stmt.ExecContext(ctx, tx); err != nil {
		return fmt.Errorf("run.postgres: StartRun: skip stale: %w", err)
	}
	return nil
}

// NOTE: a channel turn takes WhatsApp order and stops at 20 messages or 32 KiB; the rest stays pending for the next turn.
func (s *postgresStore) toTake(ctx context.Context, tx *sql.Tx, orgID, threadID uuid.UUID, channel bool) ([]uuid.UUID, error) {
	m := s.messages
	stmt := postgres.SELECT(m.ID, postgres.OCTET_LENGTH(postgres.CAST(m.Body).AS_TEXT()).AS("pending.size")).FROM(m).
		WHERE(m.ThreadID.EQ(postgres.UUID(threadID)).
			AND(m.OrgID.EQ(postgres.UUID(orgID))).
			AND(m.Status.EQ(postgres.String(string(MessagePending)))))
	if channel {
		stmt = stmt.ORDER_BY(m.ChannelTime.ASC().NULLS_LAST(), m.Seq.ASC()).LIMIT(turnMaxMessages)
	} else {
		stmt = stmt.ORDER_BY(m.Seq.ASC())
	}
	var rows []struct {
		ID   uuid.UUID `alias:"messages.id"`
		Size int       `alias:"pending.size"`
	}
	if err := stmt.QueryContext(ctx, tx, &rows); err != nil && !db.IsNoRows(err) {
		return nil, fmt.Errorf("run.postgres: StartRun: pending: %w", err)
	}
	ids := make([]uuid.UUID, 0, len(rows))
	total := 0
	for _, row := range rows {
		total += row.Size
		if channel && len(ids) > 0 && total > turnMaxBytes {
			break
		}
		ids = append(ids, row.ID)
	}
	return ids, nil
}

func (s *postgresStore) takeIDs(ctx context.Context, tx *sql.Tx, runID uuid.UUID, ids []uuid.UUID) ([]Message, error) {
	m := s.messages
	in := make([]postgres.Expression, len(ids))
	for i, id := range ids {
		in[i] = postgres.UUID(id)
	}
	take := m.UPDATE(m.Status, m.RunID).
		SET(postgres.String(string(MessageConsumed)), postgres.UUID(runID)).
		WHERE(m.ID.IN(in...)).
		RETURNING(m.AllColumns)
	var rows []messageRow
	if err := take.QueryContext(ctx, tx, &rows); err != nil {
		return nil, fmt.Errorf("run.postgres: StartRun: take pending: %w", err)
	}
	order := make(map[uuid.UUID]int, len(ids))
	for i, id := range ids {
		order[id] = i
	}
	taken := toMessages(rows)
	slices.SortFunc(taken, func(a, b Message) int { return order[a.ID] - order[b.ID] })
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
		t.ID, t.PublicID, t.OrgID, t.ProjectID, t.ThreadID, t.AgentVersionID, t.Status, t.IdempotencyKey, t.Attempt, t.IsGroup,
		t.PriceInputMicros, t.PriceOutputMicros, t.PriceCacheReadMicros, t.PriceCacheWriteMicros, t.EmbeddingPriceInputMicros,
		t.HelperPriceInputMicros, t.HelperPriceOutputMicros, t.HelperPriceCacheReadMicros, t.HelperPriceCacheWriteMicros,
		t.TranscriptionPriceMicros,
		t.StartedAt, t.CreatedAt, t.UpdatedAt,
	).VALUES(
		r.ID, r.PublicID, r.OrgID, r.ProjectID, r.ThreadID, r.AgentVersionID, string(r.Status), textOrNull(r.IdempotencyKey), r.Attempt, r.Group,
		price(func(p *Prices) *int64 { return p.Input }), price(func(p *Prices) *int64 { return p.Output }),
		price(func(p *Prices) *int64 { return p.CacheRead }), price(func(p *Prices) *int64 { return p.CacheWrite }),
		price(func(p *Prices) *int64 { return p.Embedding }),
		price(func(p *Prices) *int64 { return p.HelperInput }), price(func(p *Prices) *int64 { return p.HelperOutput }),
		price(func(p *Prices) *int64 { return p.HelperCacheRead }), price(func(p *Prices) *int64 { return p.HelperCacheWrite }),
		price(func(p *Prices) *int64 { return p.Transcription }),
		r.StartedAt.UTC(), r.StartedAt.UTC(), r.UpdatedAt.UTC(),
	)
	if _, err := stmt.ExecContext(ctx, tx); err != nil {
		return fmt.Errorf("run.postgres: insert run: %w", err)
	}
	return nil
}

// ResumeApproval moves an awaiting run back to running under token's lease, with the same attempt; it returns the run's messages.
func (s *postgresStore) ResumeApproval(ctx context.Context, token string, r *Run) ([]Message, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	var taken []Message
	err = db.InTx(ctx, s.pool, func(tx *sql.Tx) error {
		if _, err := s.lock(ctx, tx, tc.OrgID, r.ThreadID, &token); err != nil {
			return err
		}
		return s.resumeFrom(ctx, tx, tc.OrgID, r, StatusAwaitingApproval, &taken)
	})
	return taken, err
}

func (s *postgresStore) resume(ctx context.Context, tx *sql.Tx, orgID uuid.UUID, r *Run, taken *[]Message) error {
	return s.resumeFrom(ctx, tx, orgID, r, StatusInterrupted, taken)
}

func (s *postgresStore) resumeFrom(ctx context.Context, tx *sql.Tx, orgID uuid.UUID, r *Run, from Status, taken *[]Message) error {
	t := s.runs
	stmt := t.UPDATE(t.Status, t.Attempt, t.UpdatedAt).
		SET(postgres.String(string(r.Status)), postgres.Int32(r.Attempt), postgres.TimestampzT(r.UpdatedAt.UTC()))
	if from == StatusAwaitingApproval {
		// NOTE: the decision is made, so a prompt still due or retrying must never go out after it.
		stmt = t.UPDATE(t.Status, t.Attempt, t.UpdatedAt, t.ReplyState, t.ReplyKind, t.ReplyNextAt).
			SET(postgres.String(string(r.Status)), postgres.Int32(r.Attempt), postgres.TimestampzT(r.UpdatedAt.UTC()),
				postgres.NULL, postgres.NULL, postgres.NULL)
	}
	stmt = stmt.
		WHERE(t.ID.EQ(postgres.UUID(r.ID)).
			AND(t.OrgID.EQ(postgres.UUID(orgID))).
			AND(t.Status.EQ(postgres.String(string(from)))))
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

// SetTakenMessageBody rewrites a message runID took and adds to its transcription seconds, under token's lease, in one transaction.
func (s *postgresStore) SetTakenMessageBody(
	ctx context.Context, token string, runID, messageID uuid.UUID, body json.RawMessage, seconds int64,
) error {
	tc, err := tenant.From(ctx)
	if err != nil {
		return err
	}
	return db.InTx(ctx, s.pool, func(tx *sql.Tx) error {
		threadID, err := s.threadOf(ctx, tx, tc.OrgID, runID)
		if err != nil {
			return err
		}
		if _, err := s.lock(ctx, tx, tc.OrgID, threadID, &token); err != nil {
			return err
		}
		m := s.messages
		res, err := m.UPDATE(m.Body).SET(jsonb(body)).
			WHERE(m.ID.EQ(postgres.UUID(messageID)).AND(m.RunID.EQ(postgres.UUID(runID))).
				AND(m.ThreadID.EQ(postgres.UUID(threadID))).AND(m.OrgID.EQ(postgres.UUID(tc.OrgID)))).
			ExecContext(ctx, tx)
		if err != nil {
			return fmt.Errorf("run.postgres: SetTakenMessageBody: message: %w", err)
		}
		if n, err := res.RowsAffected(); err != nil {
			return fmt.Errorf("run.postgres: SetTakenMessageBody: message rows: %w", err)
		} else if n != 1 {
			return fmt.Errorf("run.postgres: SetTakenMessageBody: message %s is not taken by run %s", messageID, runID)
		}
		r := s.runs
		res, err = r.UPDATE(r.TranscriptionSeconds).SET(r.TranscriptionSeconds.ADD(postgres.Int(seconds))).
			WHERE(r.ID.EQ(postgres.UUID(runID)).AND(r.OrgID.EQ(postgres.UUID(tc.OrgID)))).
			ExecContext(ctx, tx)
		if err != nil {
			return fmt.Errorf("run.postgres: SetTakenMessageBody: run: %w", err)
		}
		if n, err := res.RowsAffected(); err != nil {
			return fmt.Errorf("run.postgres: SetTakenMessageBody: run rows: %w", err)
		} else if n != 1 {
			return fmt.Errorf("run.postgres: SetTakenMessageBody: run %s not found", runID)
		}
		return nil
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
	if !r.Status.Terminal() && r.Status != StatusInterrupted && r.Status != StatusAwaitingApproval {
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
	var next = postgres.NULL
	parts, attempts := postgres.Expression(t.ReplyPartsSent), postgres.Expression(t.ReplyAttempts)
	if r.ReplyState == ReplyDue {
		// NOTE: a new due reply starts fresh; a prompt sent while the run waited must never skip or slow it.
		next, parts, attempts = postgres.TimestampzT(r.UpdatedAt.Add(replyGrace).UTC()), postgres.Int(0), postgres.Int(0)
	}
	stmt := t.UPDATE(
		t.Status, t.Attempt, t.InputTokens, t.OutputTokens, t.CacheReadTokens, t.CacheWriteTokens, t.UsageEstimated,
		t.EmbeddingTokens, t.KnowledgeRetrieveCalls, t.KnowledgeRetrieveMS, t.KnowledgePassages,
		t.HelperInputTokens, t.HelperOutputTokens, t.HelperCacheReadTokens, t.HelperCacheWriteTokens, t.TranscriptionSeconds,
		t.CostMicros, t.LatencyMS, t.ErrorCode, t.EndedAt, t.UpdatedAt, t.ReplyState, t.ReplyNextAt, t.ReplyKind,
		t.ReplyPartsSent, t.ReplyAttempts, t.NoReplyReason,
	).SET(
		postgres.String(string(r.Status)), postgres.Int32(r.Attempt), postgres.Int(u.InputTokens), postgres.Int(u.OutputTokens),
		postgres.Int(u.CacheReadTokens), postgres.Int(u.CacheWriteTokens), postgres.Bool(u.Estimated),
		postgres.Int(u.EmbeddingTokens), postgres.Int32(r.KnowledgeRetrieveCalls), postgres.Int(r.KnowledgeRetrieveMs),
		postgres.Int32(r.KnowledgePassages),
		postgres.Int(u.HelperInputTokens), postgres.Int(u.HelperOutputTokens), postgres.Int(u.HelperCacheReadTokens),
		postgres.Int(u.HelperCacheWriteTokens), postgres.Int(u.TranscriptionSeconds),
		cost, latency, textOrNull(r.ErrorCode), ended, postgres.TimestampzT(r.UpdatedAt.UTC()),
		textOrNull(r.ReplyState), next, textOrNull(r.ReplyKind), parts, attempts, textOrNull(r.NoReplyReason),
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

func bytesOrNull(b []byte) postgres.Expression {
	if b == nil {
		return postgres.NULL
	}
	return postgres.Bytea(b)
}

func (s *postgresStore) recordIdempotencyKey(ctx context.Context, tx *sql.Tx, r *Run, taken []Message) error {
	key := IdempotencyKeyOf(taken)
	if key == "" || r.IdempotencyKey != "" {
		return nil
	}
	t := s.runs
	stmt := t.UPDATE(t.IdempotencyKey).SET(postgres.String(key)).WHERE(t.ID.EQ(postgres.UUID(r.ID)))
	if _, err := rowsOf(stmt.ExecContext(ctx, tx)); err != nil {
		return err
	}
	r.IdempotencyKey = key
	return nil
}

// ReplyProgress records that the first parts of a run's reply were sent.
func (s *postgresStore) ReplyProgress(ctx context.Context, runID uuid.UUID, kind string, partsSent int) error {
	t := s.runs
	return s.updateReply(ctx, "ReplyProgress", runID, kind, t.UPDATE(t.ReplyPartsSent).SET(postgres.Int(int64(partsSent))))
}

// ReplyRetry counts a failed send of a due reply and sets its next try.
func (s *postgresStore) ReplyRetry(ctx context.Context, runID uuid.UUID, kind string, next time.Time) error {
	t := s.runs
	return s.updateReply(ctx, "ReplyRetry", runID, kind,
		t.UPDATE(t.ReplyAttempts, t.ReplyNextAt).SET(t.ReplyAttempts.ADD(postgres.Int(1)), postgres.TimestampzT(next.UTC())))
}

// ReplyEnd ends a due reply as ReplySent or ReplyFailed.
func (s *postgresStore) ReplyEnd(ctx context.Context, runID uuid.UUID, kind, state string) error {
	t := s.runs
	return s.updateReply(ctx, "ReplyEnd", runID, kind, t.UPDATE(t.ReplyState, t.ReplyNextAt).SET(postgres.String(state), postgres.NULL))
}

// SECURITY: an update matches only the due reply of the kind its job read, so a slow prompt send never ends or advances the final reply.
func (s *postgresStore) updateReply(ctx context.Context, op string, runID uuid.UUID, kind string, stmt postgres.UpdateStatement) error {
	tc, err := tenant.From(ctx)
	if err != nil {
		return err
	}
	t := s.runs
	sameKind := t.ReplyKind.IS_NULL()
	if kind != "" {
		sameKind = t.ReplyKind.EQ(postgres.String(kind))
	}
	stmt = stmt.WHERE(t.ID.EQ(postgres.UUID(runID)).AND(t.OrgID.EQ(postgres.UUID(tc.OrgID))).
		AND(t.ReplyState.EQ(postgres.String(ReplyDue))).AND(sameKind))
	return db.Write(ctx, s.pool, func(q qrm.DB) error {
		if _, err := stmt.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("run.postgres: %s: %w", op, err)
		}
		return nil
	})
}

// AnswerMessage consumes pending message msgID into runID while token holds the thread's lease.
func (s *postgresStore) AnswerMessage(ctx context.Context, token string, threadID, msgID, runID uuid.UUID) error {
	tc, err := tenant.From(ctx)
	if err != nil {
		return err
	}
	return db.InTx(ctx, s.pool, func(tx *sql.Tx) error {
		if _, err := s.lock(ctx, tx, tc.OrgID, threadID, &token); err != nil {
			return err
		}
		m := s.messages
		stmt := m.UPDATE(m.Status, m.RunID).SET(postgres.String(string(MessageConsumed)), postgres.UUID(runID)).
			WHERE(m.ID.EQ(postgres.UUID(msgID)).
				AND(m.OrgID.EQ(postgres.UUID(tc.OrgID))).
				AND(m.ThreadID.EQ(postgres.UUID(threadID))).
				AND(m.Status.EQ(postgres.String(string(MessagePending)))))
		res, err := stmt.ExecContext(ctx, tx)
		if err != nil {
			return fmt.Errorf("run.postgres: AnswerMessage: %w", err)
		}
		if n, err := res.RowsAffected(); err != nil || n != 1 {
			return ErrNothingPending
		}
		return nil
	})
}

// HoldLease takes the thread lock with token in ctx's unit of work, failing with a *LeaseLostError when the lease moved.
func (s *postgresStore) HoldLease(ctx context.Context, token string, threadID uuid.UUID) error {
	tc, err := tenant.From(ctx)
	if err != nil {
		return err
	}
	return db.InTx(ctx, s.pool, func(tx *sql.Tx) error {
		_, err := s.lock(ctx, tx, tc.OrgID, threadID, &token)
		return err
	})
}
