package agent

import (
	"context"
	"fmt"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
)

// ByID loads an agent by id.
func (s *postgresStore) ByID(ctx context.Context, id uuid.UUID) (*Agent, error) {
	stmt := postgres.SELECT(s.agents.AllColumns).
		FROM(s.agents).
		WHERE(s.agents.ID.EQ(postgres.UUID(id))).
		LIMIT(1)
	var row agentRow
	if err := stmt.QueryContext(ctx, db.Reader(ctx, s.pool), &row); err != nil {
		if db.IsNoRows(err) {
			return nil, &NotFoundError{ID: id.String()}
		}
		return nil, fmt.Errorf("agent.postgres: ByID: %w", err)
	}
	return row.toAgent(), nil
}

// ByPublicID loads an agent by its public id.
func (s *postgresStore) ByPublicID(ctx context.Context, publicID string) (*Agent, error) {
	stmt := postgres.SELECT(s.agents.AllColumns).
		FROM(s.agents).
		WHERE(s.agents.PublicID.EQ(postgres.String(publicID))).
		LIMIT(1)
	var row agentRow
	if err := stmt.QueryContext(ctx, db.Reader(ctx, s.pool), &row); err != nil {
		if db.IsNoRows(err) {
			return nil, &NotFoundError{ID: publicID}
		}
		return nil, fmt.Errorf("agent.postgres: ByPublicID: %w", err)
	}
	return row.toAgent(), nil
}

// List returns a project's agents ordered by creation time.
func (s *postgresStore) List(ctx context.Context, projectID uuid.UUID) ([]*Agent, error) {
	stmt := postgres.SELECT(s.agents.AllColumns).
		FROM(s.agents).
		WHERE(s.agents.ProjectID.EQ(postgres.UUID(projectID))).
		ORDER_BY(s.agents.CreatedAt.ASC())
	var rows []agentRow
	if err := stmt.QueryContext(ctx, db.Reader(ctx, s.pool), &rows); err != nil {
		return nil, fmt.Errorf("agent.postgres: List: %w", err)
	}
	out := make([]*Agent, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toAgent())
	}
	return out, nil
}
