package webhook

import (
	"context"
	"fmt"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/tenant"
)

// ByID returns the endpoint in the caller's org, or a *NotFoundError.
func (s *postgresStore) ByID(ctx context.Context, id uuid.UUID) (*Endpoint, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	stmt := postgres.SELECT(s.endpoints.AllColumns).
		FROM(s.endpoints).
		WHERE(s.endpoints.ID.EQ(postgres.UUID(id)).
			AND(s.endpoints.OrgID.EQ(postgres.UUID(tc.OrgID)))).
		LIMIT(1)
	var row endpointRow
	if err := stmt.QueryContext(ctx, db.Reader(ctx, s.pool), &row); err != nil {
		if db.IsNoRows(err) {
			return nil, &NotFoundError{ID: id.String()}
		}
		return nil, fmt.Errorf("webhook.postgres.ByID: %w", err)
	}
	return row.toEndpoint()
}

// ByPublicID returns the endpoint with publicID in the caller's org, or a *NotFoundError.
func (s *postgresStore) ByPublicID(ctx context.Context, publicID string) (*Endpoint, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	stmt := postgres.SELECT(s.endpoints.AllColumns).
		FROM(s.endpoints).
		WHERE(s.endpoints.PublicID.EQ(postgres.String(publicID)).
			AND(s.endpoints.OrgID.EQ(postgres.UUID(tc.OrgID)))).
		LIMIT(1)
	var row endpointRow
	if err := stmt.QueryContext(ctx, db.Reader(ctx, s.pool), &row); err != nil {
		if db.IsNoRows(err) {
			return nil, &NotFoundError{ID: publicID}
		}
		return nil, fmt.Errorf("webhook.postgres.ByPublicID: %w", err)
	}
	return row.toEndpoint()
}

// ListByOrg returns the endpoints of every project in orgID, oldest first.
func (s *postgresStore) ListByOrg(ctx context.Context, orgID uuid.UUID) ([]*Endpoint, error) {
	stmt := postgres.SELECT(s.endpoints.AllColumns).
		FROM(s.endpoints).
		WHERE(s.endpoints.OrgID.EQ(postgres.UUID(orgID))).
		ORDER_BY(s.endpoints.CreatedAt.ASC(), s.endpoints.ID.ASC())
	var rows []endpointRow
	if err := stmt.QueryContext(ctx, db.Reader(ctx, s.pool), &rows); err != nil {
		return nil, fmt.Errorf("webhook.postgres.ListByOrg: %w", err)
	}
	out := make([]*Endpoint, 0, len(rows))
	for i := range rows {
		e, err := rows[i].toEndpoint()
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

// List returns the project's endpoints, newest first.
func (s *postgresStore) List(ctx context.Context, orgID, projectID uuid.UUID) ([]*Endpoint, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	stmt := postgres.SELECT(s.endpoints.AllColumns).
		FROM(s.endpoints).
		WHERE(s.endpoints.OrgID.EQ(postgres.UUID(orgID)).
			AND(s.endpoints.OrgID.EQ(postgres.UUID(tc.OrgID))).
			AND(s.endpoints.ProjectID.EQ(postgres.UUID(projectID)))).
		ORDER_BY(s.endpoints.CreatedAt.DESC(), s.endpoints.ID.DESC())
	var rows []endpointRow
	if err := stmt.QueryContext(ctx, db.Reader(ctx, s.pool), &rows); err != nil {
		return nil, fmt.Errorf("webhook.postgres.List: %w", err)
	}
	out := make([]*Endpoint, 0, len(rows))
	for i := range rows {
		e, err := rows[i].toEndpoint()
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

// ListAttempts returns the attempts of one delivery to one endpoint, newest first.
func (s *postgresStore) ListAttempts(ctx context.Context, endpointID, deliveryID uuid.UUID) ([]Attempt, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	stmt := postgres.SELECT(s.attempts.AllColumns).
		FROM(s.attempts).
		WHERE(s.attempts.OrgID.EQ(postgres.UUID(tc.OrgID)).
			AND(s.attempts.EndpointID.EQ(postgres.UUID(endpointID))).
			AND(s.attempts.DeliveryID.EQ(postgres.UUID(deliveryID)))).
		ORDER_BY(s.attempts.CreatedAt.DESC(), s.attempts.ID.DESC())
	var rows []attemptRow
	if err := stmt.QueryContext(ctx, db.Reader(ctx, s.pool), &rows); err != nil {
		return nil, fmt.Errorf("webhook.postgres.ListAttempts: %w", err)
	}
	out := make([]Attempt, 0, len(rows))
	for i := range rows {
		a, err := rows[i].toAttempt()
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}
