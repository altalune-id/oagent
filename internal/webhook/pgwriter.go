package webhook

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/schema/entity"
)

// Save upserts e, writing its secrets only on insert.
func (s *postgresStore) Save(ctx context.Context, e *Endpoint) error {
	tc, err := tenant.From(ctx)
	if err != nil {
		return err
	}
	return db.InTx(ctx, s.pool, func(tx *sql.Tx) error { return s.save(ctx, tx, tc, e) })
}

func (s *postgresStore) save(ctx context.Context, tx *sql.Tx, tc tenant.Context, e *Endpoint) error {
	if e.OrgID != tc.OrgID {
		// SECURITY: without RLS in force, a fresh row naming another org would otherwise land in it.
		return &NotFoundError{ID: e.ID.String()}
	}
	types, err := marshalEventTypes(e.EventTypes)
	if err != nil {
		return err
	}
	stmt := s.endpoints.INSERT(s.endpoints.AllColumns).
		VALUES(
			e.ID, e.PublicID, e.OrgID, e.ProjectID,
			e.URL, e.Description, types,
			postgres.Bytea(e.Secrets.Primary),
			pgSecondary(e.Secrets.Secondary),
			e.Active,
			e.CreatedAt.UTC(), e.UpdatedAt.UTC(),
		).
		ON_CONFLICT(s.endpoints.ID).
		// SECURITY: the conflict clause is guarded by org, or an upsert carrying another tenant's row id would rewrite that row.
		DO_UPDATE(
			postgres.SET(
				s.endpoints.URL.SET(postgres.String(e.URL)),
				s.endpoints.Description.SET(postgres.String(e.Description)),
				s.endpoints.EventTypes.SET(postgres.String(types)),
				s.endpoints.Active.SET(postgres.Bool(e.Active)),
				s.endpoints.UpdatedAt.SET(postgres.TimestampzT(e.UpdatedAt.UTC())),
			).WHERE(s.endpoints.OrgID.EQ(postgres.UUID(tc.OrgID))),
		)
	res, err := stmt.ExecContext(ctx, tx)
	if err != nil {
		return fmt.Errorf("webhook.postgres.Save: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("webhook.postgres.Save: rows affected: %w", err)
	}
	// NOTE: zero rows means the conflict clause's org guard refused.
	if n == 0 {
		return &NotFoundError{ID: e.ID.String()}
	}
	return nil
}

// SaveSecrets replaces the endpoint's secrets when the stored pair still equals expected.
func (s *postgresStore) SaveSecrets(ctx context.Context, id uuid.UUID, expected, next SealedSecrets) error {
	tc, err := tenant.From(ctx)
	if err != nil {
		return err
	}
	return db.InTx(ctx, s.pool, func(tx *sql.Tx) error { return s.saveSecrets(ctx, tx, tc, id, expected, next) })
}

func (s *postgresStore) saveSecrets(ctx context.Context, tx *sql.Tx, tc tenant.Context, id uuid.UUID, expected, next SealedSecrets) error {
	stmt := s.endpoints.UPDATE().
		SET(
			s.endpoints.SecretPrimary.SET(postgres.Bytea(next.Primary)),
			s.endpoints.SecretSecondary.SET(pgSecondary(next.Secondary)),
			s.endpoints.UpdatedAt.SET(postgres.TimestampzT(time.Now().UTC())),
		).
		WHERE(s.endpoints.OrgID.EQ(postgres.UUID(tc.OrgID)).
			AND(s.endpoints.ID.EQ(postgres.UUID(id))).
			AND(s.endpoints.SecretPrimary.EQ(postgres.Bytea(expected.Primary))).
			AND(s.endpoints.SecretSecondary.IS_NOT_DISTINCT_FROM(pgSecondary(expected.Secondary))))
	res, err := stmt.ExecContext(ctx, tx)
	if err != nil {
		return fmt.Errorf("webhook.postgres.SaveSecrets: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("webhook.postgres.SaveSecrets: rows affected: %w", err)
	}
	if n > 0 {
		return nil
	}
	probe := postgres.SELECT(s.endpoints.ID).
		FROM(s.endpoints).
		WHERE(s.endpoints.OrgID.EQ(postgres.UUID(tc.OrgID)).
			AND(s.endpoints.ID.EQ(postgres.UUID(id))))
	var row struct {
		ID uuid.UUID `alias:"webhook_endpoints.id"`
	}
	if err := probe.QueryContext(ctx, tx, &row); err != nil {
		if db.IsNoRows(err) {
			return &NotFoundError{ID: id.String()}
		}
		return fmt.Errorf("webhook.postgres.SaveSecrets: probe: %w", err)
	}
	return &SecretConflictError{ID: id.String()}
}

// Delete removes the endpoint and its attempts, or returns a *NotFoundError.
func (s *postgresStore) Delete(ctx context.Context, id uuid.UUID) error {
	tc, err := tenant.From(ctx)
	if err != nil {
		return err
	}
	return db.InTx(ctx, s.pool, func(tx *sql.Tx) error { return s.delete(ctx, tx, tc, id) })
}

func (s *postgresStore) delete(ctx context.Context, tx *sql.Tx, tc tenant.Context, id uuid.UUID) error {
	stmt := s.endpoints.DELETE().
		WHERE(s.endpoints.ID.EQ(postgres.UUID(id)).
			AND(s.endpoints.OrgID.EQ(postgres.UUID(tc.OrgID))))
	res, err := stmt.ExecContext(ctx, tx)
	if err != nil {
		return fmt.Errorf("webhook.postgres.Delete: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("webhook.postgres.Delete: rows affected: %w", err)
	}
	if n == 0 {
		return &NotFoundError{ID: id.String()}
	}
	return nil
}

// SaveAttempt appends a, returning an *InvalidAttemptError on a scope mismatch or a *NotFoundError when the endpoint is gone.
func (s *postgresStore) SaveAttempt(ctx context.Context, a Attempt) error {
	tc, err := tenant.From(ctx)
	if err != nil {
		return err
	}
	return db.InTx(ctx, s.pool, func(tx *sql.Tx) error { return s.saveAttempt(ctx, tx, tc, a) })
}

func (s *postgresStore) saveAttempt(ctx context.Context, tx *sql.Tx, tc tenant.Context, a Attempt) error {
	a, err := prepareAttempt(tc, a)
	if err != nil {
		return err
	}
	headers, err := marshalHeaders(a.ResponseHeaders)
	if err != nil {
		return err
	}
	stmt := s.attempts.INSERT(s.attempts.AllColumns).
		VALUES(
			a.ID, a.OrgID, a.ProjectID, a.EndpointID, a.DeliveryID, a.EventID,
			string(a.EventType), a.Attempt, a.StatusCode, a.Error,
			a.ResponseBody, a.ResponseTruncated, headers,
			a.Duration.Milliseconds(), a.CreatedAt,
		)
	if _, err := stmt.ExecContext(ctx, tx); err != nil {
		if db.IsForeignKeyViolation(err) {
			return &NotFoundError{ID: a.EndpointID.String()}
		}
		return fmt.Errorf("webhook.postgres.SaveAttempt: %w", err)
	}
	return nil
}

func pgSecondary(b []byte) postgres.ByteaExpression {
	if len(b) == 0 {
		return entity.NullBytea()
	}
	return postgres.Bytea(b)
}
