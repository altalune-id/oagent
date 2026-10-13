package mailqueue

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/go-jet/jet/v2/qrm"
	"github.com/google/uuid"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/mailer"
	"altalune.id/oagent/schema/entity"
)

// Enqueue seals m and stores it pending; it joins the transaction on ctx when there is one.
func (s *postgresStore) Enqueue(ctx context.Context, m mailer.Message) (uuid.UUID, error) {
	if err := m.Validate(); err != nil {
		return uuid.Nil, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.Nil, fmt.Errorf("mailqueue: id: %w", err)
	}
	payload, sealed, err := s.seal(id, m)
	if err != nil {
		return uuid.Nil, err
	}
	now := time.Now().UTC()
	stmt := s.q.INSERT(s.q.AllColumns).
		VALUES(id, payload, sealed, string(StatusPending), 0, now, "", now, entity.NullTimestampz())
	if _, err := stmt.ExecContext(ctx, db.Writer(ctx, s.pool)); err != nil {
		return uuid.Nil, fmt.Errorf("mailqueue.postgres.Enqueue: %w", err)
	}
	return id, nil
}

// ClaimDue takes up to limit due entries exclusively, bumping Attempt and leasing each for ClaimLease.
func (s *postgresStore) ClaimDue(ctx context.Context, now time.Time, limit int) ([]Entry, error) {
	var claimed []Entry
	err := db.InTx(ctx, s.pool, func(tx *sql.Tx) error {
		var inErr error
		claimed, inErr = s.claimDue(ctx, tx, now, limit)
		return inErr
	})
	if err != nil {
		return nil, err
	}
	return claimed, nil
}

func (s *postgresStore) claimDue(ctx context.Context, tx *sql.Tx, now time.Time, limit int) ([]Entry, error) {
	if err := s.reapExhausted(ctx, tx, now); err != nil {
		return nil, err
	}
	// SECURITY/correctness: SKIP LOCKED stops two replicas claiming one entry and sending it twice on a tick.
	due := postgres.SELECT(s.q.ID).
		FROM(s.q).
		WHERE(s.q.Status.EQ(postgres.String(string(StatusPending))).
			AND(s.q.Attempt.LT(postgres.Int(MaxAttempts))).
			AND(s.q.NextAttemptAt.LT_EQ(postgres.TimestampzT(now.UTC())))).
		ORDER_BY(s.q.NextAttemptAt.ASC()).
		LIMIT(claimBatch(limit)).
		FOR(postgres.UPDATE().SKIP_LOCKED())
	// NOTE: locked first, then updated by id; an IN (subquery) may be rescanned and claim more than limit.
	var locked []struct {
		ID uuid.UUID `alias:"mail_queue.id"`
	}
	if err := due.QueryContext(ctx, tx, &locked); err != nil && !db.IsNoRows(err) {
		return nil, fmt.Errorf("mailqueue.postgres.ClaimDue: lock due: %w", err)
	}
	if len(locked) == 0 {
		return []Entry{}, nil
	}
	ids := make([]postgres.Expression, len(locked))
	for i, l := range locked {
		ids[i] = postgres.UUID(l.ID)
	}

	stmt := s.q.UPDATE(s.q.Attempt, s.q.NextAttemptAt).
		SET(s.q.Attempt.ADD(postgres.Int(1)), postgres.TimestampzT(now.UTC().Add(ClaimLease))).
		WHERE(s.q.ID.IN(ids...)).
		RETURNING(s.q.AllColumns)

	var rows []entryRow
	if err := stmt.QueryContext(ctx, tx, &rows); err != nil && !db.IsNoRows(err) {
		return nil, fmt.Errorf("mailqueue.postgres.ClaimDue: %w", err)
	}
	out := make([]Entry, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toEntry())
	}
	return out, nil
}

// NOTE: without it a sender that dies on the last attempt strands the row, and its payload, in pending.
func (s *postgresStore) reapExhausted(ctx context.Context, tx *sql.Tx, now time.Time) error {
	stmt := s.q.UPDATE(s.q.Status, s.q.Payload, s.q.SettledAt).
		SET(postgres.String(string(StatusFailed)), postgres.NULL, postgres.TimestampzT(now.UTC())).
		WHERE(s.q.Status.EQ(postgres.String(string(StatusPending))).
			AND(s.q.Attempt.GT_EQ(postgres.Int(MaxAttempts))).
			AND(s.q.NextAttemptAt.LT_EQ(postgres.TimestampzT(now.UTC()))))
	if _, err := stmt.ExecContext(ctx, tx); err != nil {
		return fmt.Errorf("mailqueue.postgres.ClaimDue: reap exhausted: %w", err)
	}
	return nil
}

// Succeed settles the claimed entry e as sent and erases its payload.
func (s *postgresStore) Succeed(ctx context.Context, e Entry, at time.Time) error {
	stmt := s.q.UPDATE(s.q.Status, s.q.Payload, s.q.SettledAt, s.q.LastError).
		SET(postgres.String(string(StatusSent)), postgres.NULL, postgres.TimestampzT(at.UTC()), postgres.String("")).
		WHERE(s.claimedRow(e))
	return s.settle(ctx, e, stmt)
}

// Fail reschedules the claimed entry e for retryAt, or settles it as failed with its payload erased at MaxAttempts.
func (s *postgresStore) Fail(ctx context.Context, e Entry, retryAt time.Time, cause string) error {
	if e.Attempt >= MaxAttempts {
		return s.GiveUp(ctx, e, retryAt, cause)
	}
	stmt := s.q.UPDATE(s.q.NextAttemptAt, s.q.LastError).
		SET(postgres.TimestampzT(retryAt.UTC()), postgres.String(apperror.TruncateCause(cause, MaxErrorLen))).
		WHERE(s.claimedRow(e))
	return s.settle(ctx, e, stmt)
}

// GiveUp settles the claimed entry e as failed at at, erasing its payload.
func (s *postgresStore) GiveUp(ctx context.Context, e Entry, at time.Time, cause string) error {
	stmt := s.q.UPDATE(s.q.Status, s.q.Payload, s.q.SettledAt, s.q.LastError).
		SET(postgres.String(string(StatusFailed)), postgres.NULL, postgres.TimestampzT(at.UTC()),
			postgres.String(apperror.TruncateCause(cause, MaxErrorLen))).
		WHERE(s.claimedRow(e))
	return s.settle(ctx, e, stmt)
}

// PruneSettled deletes sent and failed entries settled before before and returns how many it deleted.
func (s *postgresStore) PruneSettled(ctx context.Context, before time.Time) (int64, error) {
	stmt := s.q.DELETE().
		WHERE(s.q.Status.NOT_EQ(postgres.String(string(StatusPending))).
			AND(s.q.SettledAt.LT(postgres.TimestampzT(before.UTC()))))
	res, err := stmt.ExecContext(ctx, db.Writer(ctx, s.pool))
	if err != nil {
		return 0, fmt.Errorf("mailqueue.postgres.PruneSettled: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("mailqueue.postgres.PruneSettled: rows affected: %w", err)
	}
	return n, nil
}

// SECURITY/correctness: the attempt fences the settle, so a sender whose lease expired cannot overwrite a re-claim.
func (s *postgresStore) claimedRow(e Entry) postgres.BoolExpression {
	return s.q.ID.EQ(postgres.UUID(e.ID)).
		AND(s.q.Status.EQ(postgres.String(string(StatusPending)))).
		AND(s.q.Attempt.EQ(postgres.Int(int64(e.Attempt))))
}

type pgExecutable interface {
	ExecContext(ctx context.Context, db qrm.Executable) (sql.Result, error)
}

func (s *postgresStore) settle(ctx context.Context, e Entry, stmt pgExecutable) error {
	res, err := stmt.ExecContext(ctx, db.Writer(ctx, s.pool))
	if err != nil {
		return fmt.Errorf("mailqueue.postgres: settle: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("mailqueue.postgres: settle: rows affected: %w", err)
	}
	if n == 0 {
		return &SettleRefusedError{ID: e.ID.String()}
	}
	return nil
}
