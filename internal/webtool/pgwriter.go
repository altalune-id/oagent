package webtool

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/go-jet/jet/v2/qrm"
	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/tenant"
)

// SaveProvider inserts or replaces a project's search provider.
func (s *postgresStore) SaveProvider(ctx context.Context, p *Provider) error {
	tc, err := tenant.From(ctx)
	if err != nil {
		return err
	}
	if p.OrgID != tc.OrgID {
		// SECURITY: without RLS in force, a fresh row naming another org would otherwise land in it.
		return &NotFoundError{}
	}
	t := s.providers
	stmt := t.INSERT(t.AllColumns).
		VALUES(p.ProjectID, p.OrgID, string(p.Kind), postgres.Bytea(p.APIKeySealed), p.PricePerCallMicros, p.CreatedAt.UTC(), p.UpdatedAt.UTC()).
		ON_CONFLICT(t.ProjectID).
		// SECURITY: the conflict clause is guarded by org, or an upsert naming another tenant's project would rewrite its row.
		DO_UPDATE(
			postgres.SET(
				t.Kind.SET(postgres.String(string(p.Kind))),
				t.APIKeySealed.SET(postgres.Bytea(p.APIKeySealed)),
				t.PricePerCallMicros.SET(postgres.Int64(p.PricePerCallMicros)),
				t.UpdatedAt.SET(postgres.TimestampzT(p.UpdatedAt.UTC())),
			).WHERE(t.OrgID.EQ(postgres.UUID(tc.OrgID))),
		)
	var res sql.Result
	err = db.Write(ctx, s.pool, func(q qrm.DB) (err error) {
		res, err = stmt.ExecContext(ctx, q)
		return err
	})
	if err != nil {
		if db.IsRowSecurityViolation(err) || db.IsForeignKeyViolation(err) {
			return &NotFoundError{}
		}
		return fmt.Errorf("webtool.postgres: SaveProvider: %w", err)
	}
	// NOTE: zero rows means the conflict clause's org guard refused.
	if n, rowsErr := res.RowsAffected(); rowsErr == nil && n == 0 {
		return &NotFoundError{}
	}
	return nil
}

// DeleteProvider removes a project's search provider; a project with none is not an error.
func (s *postgresStore) DeleteProvider(ctx context.Context, projectID uuid.UUID) error {
	tc, err := tenant.From(ctx)
	if err != nil {
		return err
	}
	t := s.providers
	stmt := t.DELETE().WHERE(t.ProjectID.EQ(postgres.UUID(projectID)).AND(t.OrgID.EQ(postgres.UUID(tc.OrgID))))
	if err := db.Write(ctx, s.pool, func(q qrm.DB) error { _, err := stmt.ExecContext(ctx, q); return err }); err != nil {
		return fmt.Errorf("webtool.postgres: DeleteProvider: %w", err)
	}
	return nil
}

// SaveSecret inserts a secret or replaces the value of the project's secret for the same host and header, returning the stored row.
func (s *postgresStore) SaveSecret(ctx context.Context, sec *HTTPSecret) (*HTTPSecret, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	if sec.OrgID != tc.OrgID {
		// SECURITY: without RLS in force, a fresh row naming another org would otherwise land in it.
		return nil, &NotFoundError{}
	}
	t := s.secrets
	stmt := t.INSERT(t.AllColumns).
		VALUES(
			sec.ID, sec.PublicID, sec.OrgID, sec.ProjectID, sec.Host, sec.Header,
			postgres.Bytea(sec.ValueSealed), sec.CreatedAt.UTC(), sec.UpdatedAt.UTC(),
		).
		ON_CONFLICT(t.ProjectID, t.Host, t.Header).
		// SECURITY: the conflict clause is guarded by org, or an upsert naming another tenant's project would rewrite its secret.
		DO_UPDATE(
			postgres.SET(
				t.ValueSealed.SET(postgres.Bytea(sec.ValueSealed)),
				t.UpdatedAt.SET(postgres.TimestampzT(sec.UpdatedAt.UTC())),
			).WHERE(t.OrgID.EQ(postgres.UUID(tc.OrgID))),
		).
		RETURNING(t.AllColumns)
	var row secretRow
	err = db.Write(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &row) })
	if err != nil {
		// NOTE: no row back means the conflict clause's org guard refused.
		if db.IsNoRows(err) || db.IsRowSecurityViolation(err) || db.IsForeignKeyViolation(err) {
			return nil, &NotFoundError{}
		}
		return nil, fmt.Errorf("webtool.postgres: SaveSecret: %w", err)
	}
	return row.toSecret(), nil
}

// DeleteSecret removes a secret of the org on ctx by id.
func (s *postgresStore) DeleteSecret(ctx context.Context, id uuid.UUID) error {
	tc, err := tenant.From(ctx)
	if err != nil {
		return err
	}
	t := s.secrets
	stmt := t.DELETE().WHERE(t.ID.EQ(postgres.UUID(id)).AND(t.OrgID.EQ(postgres.UUID(tc.OrgID))))
	var res sql.Result
	err = db.Write(ctx, s.pool, func(q qrm.DB) (err error) {
		res, err = stmt.ExecContext(ctx, q)
		return err
	})
	if err != nil {
		return fmt.Errorf("webtool.postgres: DeleteSecret: %w", err)
	}
	if n, rowsErr := res.RowsAffected(); rowsErr == nil && n == 0 {
		return &NotFoundError{}
	}
	return nil
}
