package modelprovider

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/go-jet/jet/v2/qrm"
	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/tenant"
)

var errLockOutsideTx = errors.New("modelprovider.postgres: a row lock needs a transaction")

// Save inserts a provider or updates its editable fields and sealed key; a name taken in the project is an *AlreadyExistsError.
func (s *postgresStore) Save(ctx context.Context, p *Provider) error {
	tc, err := tenant.From(ctx)
	if err != nil {
		return err
	}
	if p.OrgID != tc.OrgID {
		// SECURITY: without RLS in force, a fresh row naming another org would otherwise land in it.
		return &NotFoundError{ID: p.ID.String()}
	}
	models, err := json.Marshal(p.Models)
	if err != nil {
		return fmt.Errorf("modelprovider.postgres: Save: %w", err)
	}
	modelsExpr := postgres.StringExp(postgres.CAST(postgres.String(string(models))).AS("jsonb"))
	t := s.providers
	stmt := t.INSERT(t.AllColumns).
		VALUES(
			p.ID, p.PublicID, p.OrgID, p.ProjectID, p.Name, string(p.Kind), p.BaseURL,
			postgres.Bytea(p.APIKeySealed), modelsExpr, p.CreatedAt.UTC(), p.UpdatedAt.UTC(),
		).
		ON_CONFLICT(t.ID).
		// SECURITY: the conflict clause is guarded by org, or an upsert carrying another tenant's row id would rewrite that row.
		DO_UPDATE(
			postgres.SET(
				t.Name.SET(postgres.String(p.Name)),
				t.Kind.SET(postgres.String(string(p.Kind))),
				t.BaseURL.SET(postgres.String(p.BaseURL)),
				t.APIKeySealed.SET(postgres.Bytea(p.APIKeySealed)),
				t.Models.SET(modelsExpr),
				t.UpdatedAt.SET(postgres.TimestampzT(p.UpdatedAt.UTC())),
			).WHERE(t.OrgID.EQ(postgres.UUID(tc.OrgID))),
		)
	var res sql.Result
	err = db.Write(ctx, s.pool, func(q qrm.DB) (err error) {
		res, err = stmt.ExecContext(ctx, q)
		return err
	})
	if err != nil {
		if db.IsUniqueViolation(err) {
			return &AlreadyExistsError{Name: p.Name}
		}
		if db.IsRowSecurityViolation(err) {
			return &NotFoundError{ID: p.ID.String()}
		}
		return fmt.Errorf("modelprovider.postgres: Save: %w", err)
	}
	// NOTE: zero rows means the conflict clause's org guard refused.
	if n, rowsErr := res.RowsAffected(); rowsErr == nil && n == 0 {
		return &NotFoundError{ID: p.ID.String()}
	}
	return nil
}

// Delete removes a provider of the org on ctx by id.
func (s *postgresStore) Delete(ctx context.Context, id uuid.UUID) error {
	tc, err := tenant.From(ctx)
	if err != nil {
		return err
	}
	t := s.providers
	stmt := t.DELETE().WHERE(t.ID.EQ(postgres.UUID(id)).AND(t.OrgID.EQ(postgres.UUID(tc.OrgID))))
	var res sql.Result
	err = db.Write(ctx, s.pool, func(q qrm.DB) (err error) {
		res, err = stmt.ExecContext(ctx, q)
		return err
	})
	if err != nil {
		return fmt.Errorf("modelprovider.postgres: Delete: %w", err)
	}
	if n, rowsErr := res.RowsAffected(); rowsErr == nil && n == 0 {
		return &NotFoundError{ID: id.String()}
	}
	return nil
}

// LockForUpdate loads a provider FOR UPDATE, so a concurrent LockShared or LockForUpdate on it waits for the unit of work on ctx.
func (s *postgresStore) LockForUpdate(ctx context.Context, id uuid.UUID) (*Provider, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	tx, ok := db.CurrentTx(ctx)
	if !ok {
		return nil, errLockOutsideTx
	}
	t := s.providers
	stmt := postgres.SELECT(t.AllColumns).
		FROM(t).
		WHERE(t.ID.EQ(postgres.UUID(id)).AND(t.OrgID.EQ(postgres.UUID(tc.OrgID)))).
		FOR(postgres.UPDATE())
	var row providerRow
	if err := stmt.QueryContext(ctx, tx, &row); err != nil {
		if db.IsNoRows(err) {
			return nil, &NotFoundError{ID: id.String()}
		}
		return nil, fmt.Errorf("modelprovider.postgres: LockForUpdate: %w", err)
	}
	return row.toProvider()
}

// LockShared locks the providers FOR SHARE until the unit of work on ctx ends; a missing one is a *NotFoundError.
func (s *postgresStore) LockShared(ctx context.Context, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	tc, err := tenant.From(ctx)
	if err != nil {
		return err
	}
	tx, ok := db.CurrentTx(ctx)
	if !ok {
		return errLockOutsideTx
	}
	t := s.providers
	in := make([]postgres.Expression, len(ids))
	for i, id := range ids {
		in[i] = postgres.UUID(id)
	}
	stmt := postgres.SELECT(t.ID).
		FROM(t).
		WHERE(t.ID.IN(in...).AND(t.OrgID.EQ(postgres.UUID(tc.OrgID)))).
		ORDER_BY(t.ID.ASC()).
		FOR(postgres.SHARE())
	var found []idRow
	if err := stmt.QueryContext(ctx, tx, &found); err != nil {
		return fmt.Errorf("modelprovider.postgres: LockShared: %w", err)
	}
	for _, id := range ids {
		if !slices.Contains(found, idRow{ID: id}) {
			return &NotFoundError{ID: id.String()}
		}
	}
	return nil
}

type idRow struct {
	ID uuid.UUID `alias:"model_providers.id"`
}
