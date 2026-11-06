package modelprovider

import (
	"context"
	"fmt"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/go-jet/jet/v2/qrm"
	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/tenant"
)

// ByID loads a provider of the org on ctx by id.
func (s *postgresStore) ByID(ctx context.Context, id uuid.UUID) (*Provider, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	t := s.providers
	stmt := postgres.SELECT(t.AllColumns).
		FROM(t).
		WHERE(t.ID.EQ(postgres.UUID(id)).AND(t.OrgID.EQ(postgres.UUID(tc.OrgID)))).
		LIMIT(1)
	var row providerRow
	if err := db.Read(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &row) }); err != nil {
		if db.IsNoRows(err) {
			return nil, &NotFoundError{ID: id.String()}
		}
		return nil, fmt.Errorf("modelprovider.postgres: ByID: %w", err)
	}
	return row.toProvider()
}

// List returns a project's providers, oldest first.
func (s *postgresStore) List(ctx context.Context, projectID uuid.UUID) ([]*Provider, error) {
	t := s.providers
	stmt := postgres.SELECT(t.AllColumns).
		FROM(t).
		WHERE(t.ProjectID.EQ(postgres.UUID(projectID))).
		ORDER_BY(t.CreatedAt.ASC(), t.ID.ASC())
	var rows []providerRow
	if err := db.Read(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &rows) }); err != nil {
		return nil, fmt.Errorf("modelprovider.postgres: List: %w", err)
	}
	return toProviders(rows)
}

// Resolve names publicID's provider and its org through a definer, so a caller with no active org can check reach and then scope.
func (s *postgresStore) Resolve(ctx context.Context, publicID string) (Ref, error) {
	var row struct {
		ID        uuid.UUID `alias:"model_providers.id"`
		OrgID     uuid.UUID `alias:"model_providers.org_id"`
		ProjectID uuid.UUID `alias:"model_providers.project_id"`
	}
	stmt := postgres.RawStatement(s.resolve, postgres.RawArgs{"#publicID": publicID})
	if err := db.Read(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &row) }); err != nil {
		if db.IsNoRows(err) {
			return Ref{}, &NotFoundError{}
		}
		return Ref{}, fmt.Errorf("modelprovider.postgres: Resolve: %w", err)
	}
	return Ref(row), nil
}
