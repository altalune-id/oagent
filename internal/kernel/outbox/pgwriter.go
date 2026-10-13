package outbox

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/go-jet/jet/v2/qrm"
	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/schema/entity"
)

// Enqueue records a pending entry, ignoring a repeat of an (EventID, Target) already queued.
func (s *postgresStore) Enqueue(ctx context.Context, e Entry) error {
	if err := e.validate(); err != nil {
		return err
	}
	tc, err := tenant.From(ctx)
	if err != nil {
		return err
	}
	return db.InTx(ctx, s.pool, func(tx *sql.Tx) error { return s.enqueue(ctx, tx, tc, e) })
}

func (s *postgresStore) enqueue(ctx context.Context, tx *sql.Tx, tc tenant.Context, e Entry) error {
	if e.OrgID != tc.OrgID {
		// SECURITY: an entry naming another org would be delivered under that org's scope.
		return &InvalidEntryError{Field: "OrgID", Reason: "does not match the tenant scope on ctx"}
	}
	if e.ProjectID != tc.ProjectID {
		return &InvalidEntryError{Field: "ProjectID", Reason: "does not match the tenant scope on ctx"}
	}
	now := time.Now().UTC()
	due := e.NextAttemptAt
	if due.IsZero() {
		due = now
	}
	stmt := s.entries.INSERT(s.entries.AllColumns).
		VALUES(
			e.ID, e.EventID, e.OrgID, e.ProjectID, e.Target, e.Payload,
			0, due.UTC(), string(StatusPending), "", now, entity.NullTimestampz(), e.PublicID, e.EventPublicID,
		).
		ON_CONFLICT(s.entries.OrgID, s.entries.EventID, s.entries.Target).
		// NOTE: a redelivered domain event must not become a second delivery.
		DO_NOTHING()
	if _, execErr := stmt.ExecContext(ctx, tx); execErr != nil {
		return fmt.Errorf("outbox.postgres.Enqueue: %w", execErr)
	}
	return nil
}

// ClaimDue takes up to limit due entries exclusively, bumping Attempt and leasing each for ClaimLease.
func (s *postgresStore) ClaimDue(ctx context.Context, now time.Time, limit int) ([]Entry, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	var claimed []Entry
	err = db.InTx(ctx, s.pool, func(tx *sql.Tx) error {
		var inErr error
		claimed, inErr = s.claimDue(ctx, tx, tc, now, limit)
		return inErr
	})
	if err != nil {
		return nil, err
	}
	return claimed, nil
}

func (s *postgresStore) claimDue(ctx context.Context, tx *sql.Tx, tc tenant.Context, now time.Time, limit int) ([]Entry, error) {
	if err := s.reapExhausted(ctx, tx, tc, now); err != nil {
		return nil, err
	}
	// SECURITY/correctness: SKIP LOCKED stops two replicas claiming one entry and double-delivering on a tick.
	due := postgres.SELECT(s.entries.ID).
		FROM(s.entries).
		WHERE(s.entries.OrgID.EQ(postgres.UUID(tc.OrgID)).
			AND(s.entries.Status.EQ(postgres.String(string(StatusPending)))).
			AND(s.entries.Attempt.LT(postgres.Int(MaxAttempts))).
			AND(s.entries.NextAttemptAt.LT_EQ(postgres.TimestampzT(now.UTC())))).
		ORDER_BY(s.entries.NextAttemptAt.ASC()).
		LIMIT(claimBatch(limit)).
		FOR(postgres.UPDATE().SKIP_LOCKED())
	// NOTE: locked first, then updated by id; an IN (subquery) may be rescanned and claim more than limit.
	var locked []struct {
		ID uuid.UUID `alias:"outbox_entries.id"`
	}
	if err := due.QueryContext(ctx, tx, &locked); err != nil && !db.IsNoRows(err) {
		return nil, fmt.Errorf("outbox.postgres.ClaimDue: lock due: %w", err)
	}
	if len(locked) == 0 {
		return []Entry{}, nil
	}
	ids := make([]postgres.Expression, len(locked))
	for i, l := range locked {
		ids[i] = postgres.UUID(l.ID)
	}

	stmt := s.entries.UPDATE(s.entries.Attempt, s.entries.NextAttemptAt).
		SET(
			s.entries.Attempt.ADD(postgres.Int(1)),
			postgres.TimestampzT(now.UTC().Add(ClaimLease)),
		).
		WHERE(s.entries.OrgID.EQ(postgres.UUID(tc.OrgID)).AND(s.entries.ID.IN(ids...))).
		RETURNING(s.entries.AllColumns)

	var rows []entryRow
	if err := stmt.QueryContext(ctx, tx, &rows); err != nil && !db.IsNoRows(err) {
		return nil, fmt.Errorf("outbox.postgres.ClaimDue: %w", err)
	}
	out := make([]Entry, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toEntry())
	}
	return out, nil
}

// NOTE: without it a dispatcher that dies mid-flight every time strands the row in pending.
func (s *postgresStore) reapExhausted(ctx context.Context, tx *sql.Tx, tc tenant.Context, now time.Time) error {
	stmt := s.entries.UPDATE(s.entries.Status).
		SET(postgres.String(string(StatusFailed))).
		WHERE(s.entries.OrgID.EQ(postgres.UUID(tc.OrgID)).
			AND(s.entries.Status.EQ(postgres.String(string(StatusPending)))).
			AND(s.entries.Attempt.GT_EQ(postgres.Int(MaxAttempts))).
			AND(s.entries.NextAttemptAt.LT_EQ(postgres.TimestampzT(now.UTC()))))
	if _, err := stmt.ExecContext(ctx, tx); err != nil {
		return fmt.Errorf("outbox.postgres.ClaimDue: reap exhausted: %w", err)
	}
	return nil
}

// Succeed settles the claimed entry e as delivered, refusing a settle from a superseded claim.
func (s *postgresStore) Succeed(ctx context.Context, e Entry, at time.Time) error {
	tc, err := tenant.From(ctx)
	if err != nil {
		return err
	}
	stmt := s.entries.UPDATE(s.entries.Status, s.entries.DeliveredAt, s.entries.LastError).
		SET(postgres.String(string(StatusDelivered)), postgres.TimestampzT(at.UTC()), postgres.String("")).
		WHERE(s.claimedRow(tc, e))
	return db.InTx(ctx, s.pool, func(tx *sql.Tx) error { return s.settle(ctx, tx, tc, e, stmt) })
}

// Fail reschedules the claimed entry e for retryAt, or settles it as failed once MaxAttempts is reached.
func (s *postgresStore) Fail(ctx context.Context, e Entry, retryAt time.Time, cause string) error {
	tc, err := tenant.From(ctx)
	if err != nil {
		return err
	}
	nextStatus := postgres.CASE().
		WHEN(s.entries.Attempt.GT_EQ(postgres.Int(MaxAttempts))).
		THEN(postgres.String(string(StatusFailed))).
		ELSE(postgres.String(string(StatusPending)))
	stmt := s.entries.UPDATE(s.entries.Status, s.entries.NextAttemptAt, s.entries.LastError).
		SET(nextStatus, postgres.TimestampzT(retryAt.UTC()), postgres.String(truncateCause(cause))).
		WHERE(s.claimedRow(tc, e))
	return db.InTx(ctx, s.pool, func(tx *sql.Tx) error { return s.settle(ctx, tx, tc, e, stmt) })
}

// SECURITY/correctness: the attempt fences the settle, so a dispatcher whose lease expired cannot overwrite a re-claim.
func (s *postgresStore) claimedRow(tc tenant.Context, e Entry) postgres.BoolExpression {
	return s.entries.OrgID.EQ(postgres.UUID(tc.OrgID)).
		AND(s.entries.ID.EQ(postgres.UUID(e.ID))).
		AND(s.entries.Status.EQ(postgres.String(string(StatusPending)))).
		AND(s.entries.Attempt.EQ(postgres.Int(int64(e.Attempt))))
}

type pgExecutable interface {
	ExecContext(ctx context.Context, db qrm.Executable) (sql.Result, error)
}

func (s *postgresStore) settle(ctx context.Context, tx *sql.Tx, tc tenant.Context, e Entry, stmt pgExecutable) error {
	res, err := stmt.ExecContext(ctx, tx)
	if err != nil {
		return fmt.Errorf("outbox.postgres: settle: %w", err)
	}
	n, raErr := res.RowsAffected()
	if raErr != nil {
		return fmt.Errorf("outbox.postgres: settle: rows affected: %w", raErr)
	}
	if n > 0 {
		return nil
	}
	return s.refuseReason(ctx, tx, tc, e)
}

func (s *postgresStore) refuseReason(ctx context.Context, tx *sql.Tx, tc tenant.Context, e Entry) error {
	stmt := postgres.SELECT(s.entries.Status, s.entries.Attempt).
		FROM(s.entries).
		WHERE(s.entries.OrgID.EQ(postgres.UUID(tc.OrgID)).AND(s.entries.ID.EQ(postgres.UUID(e.ID))))
	var row struct {
		Status  string `alias:"outbox_entries.status"`
		Attempt int32  `alias:"outbox_entries.attempt"`
	}
	if err := stmt.QueryContext(ctx, tx, &row); err != nil {
		if db.IsNoRows(err) {
			return &NotFoundError{ID: e.ID.String()}
		}
		return fmt.Errorf("outbox.postgres: settle: read status: %w", err)
	}
	if Status(row.Status) == StatusPending {
		return &StaleClaimError{ID: e.ID.String(), Claimed: e.Attempt, Current: int(row.Attempt)}
	}
	return &TerminalStateError{ID: e.ID.String(), Status: Status(row.Status)}
}

// Requeue resets the failed entry id/target to pending, attempt 0, for a fresh claim.
func (s *postgresStore) Requeue(ctx context.Context, id uuid.UUID, target string) error {
	tc, err := tenant.From(ctx)
	if err != nil {
		return err
	}
	return db.InTx(ctx, s.pool, func(tx *sql.Tx) error { return s.requeue(ctx, tx, tc, id, target) })
}

func (s *postgresStore) requeue(ctx context.Context, tx *sql.Tx, tc tenant.Context, id uuid.UUID, target string) error {
	stmt := s.entries.UPDATE(s.entries.Status, s.entries.Attempt, s.entries.NextAttemptAt, s.entries.LastError).
		SET(postgres.String(string(StatusPending)), postgres.Int(0), postgres.TimestampzT(time.Now().UTC()), postgres.String("")).
		WHERE(s.failedRow(tc, id, target))
	res, err := stmt.ExecContext(ctx, tx)
	if err != nil {
		return fmt.Errorf("outbox.postgres.Requeue: %w", err)
	}
	n, raErr := res.RowsAffected()
	if raErr != nil {
		return fmt.Errorf("outbox.postgres.Requeue: rows affected: %w", raErr)
	}
	if n > 0 {
		return nil
	}
	return s.refuseRequeue(ctx, tx, tc, id, target)
}

func (s *postgresStore) failedRow(tc tenant.Context, id uuid.UUID, target string) postgres.BoolExpression {
	return s.entries.OrgID.EQ(postgres.UUID(tc.OrgID)).
		AND(s.entries.ID.EQ(postgres.UUID(id))).
		AND(s.entries.Target.EQ(postgres.String(target))).
		AND(s.entries.Status.EQ(postgres.String(string(StatusFailed))))
}

func (s *postgresStore) refuseRequeue(ctx context.Context, tx *sql.Tx, tc tenant.Context, id uuid.UUID, target string) error {
	stmt := postgres.SELECT(s.entries.Status).
		FROM(s.entries).
		WHERE(s.entries.OrgID.EQ(postgres.UUID(tc.OrgID)).
			AND(s.entries.ID.EQ(postgres.UUID(id))).
			AND(s.entries.Target.EQ(postgres.String(target))))
	var row struct {
		Status string `alias:"outbox_entries.status"`
	}
	if err := stmt.QueryContext(ctx, tx, &row); err != nil {
		if db.IsNoRows(err) {
			return &NotFoundError{ID: id.String()}
		}
		return fmt.Errorf("outbox.postgres.Requeue: read status: %w", err)
	}
	return &NotFailedError{ID: id.String(), Status: Status(row.Status)}
}

// RequeueFailed resets every failed entry of target to pending, attempt 0, returning how many it touched.
func (s *postgresStore) RequeueFailed(ctx context.Context, target string) (int, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return 0, err
	}
	var n int
	err = db.InTx(ctx, s.pool, func(tx *sql.Tx) error {
		var inErr error
		n, inErr = s.requeueFailed(ctx, tx, tc, target)
		return inErr
	})
	if err != nil {
		return 0, err
	}
	return n, nil
}

func (s *postgresStore) requeueFailed(ctx context.Context, tx *sql.Tx, tc tenant.Context, target string) (int, error) {
	stmt := s.entries.UPDATE(s.entries.Status, s.entries.Attempt, s.entries.NextAttemptAt, s.entries.LastError).
		SET(postgres.String(string(StatusPending)), postgres.Int(0), postgres.TimestampzT(time.Now().UTC()), postgres.String("")).
		WHERE(s.entries.OrgID.EQ(postgres.UUID(tc.OrgID)).
			AND(s.entries.Target.EQ(postgres.String(target))).
			AND(s.entries.Status.EQ(postgres.String(string(StatusFailed)))))
	res, err := stmt.ExecContext(ctx, tx)
	if err != nil {
		return 0, fmt.Errorf("outbox.postgres.RequeueFailed: %w", err)
	}
	n, raErr := res.RowsAffected()
	if raErr != nil {
		return 0, fmt.Errorf("outbox.postgres.RequeueFailed: rows affected: %w", raErr)
	}
	return int(n), nil
}
