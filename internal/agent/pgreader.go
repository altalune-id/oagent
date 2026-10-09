package agent

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/go-jet/jet/v2/qrm"
	"github.com/google/uuid"
)

// ByID loads an agent by id.
func (s *postgresStore) ByID(ctx context.Context, id uuid.UUID) (*Agent, error) {
	stmt := postgres.SELECT(s.agents.AllColumns).
		FROM(s.agents).
		WHERE(s.agents.ID.EQ(postgres.UUID(id))).
		LIMIT(1)
	var row agentRow
	if err := stmt.QueryContext(ctx, s.reader(ctx), &row); err != nil {
		if errors.Is(err, qrm.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return nil, &NotFoundError{ID: id.String()}
		}
		return nil, fmt.Errorf("agent.postgres: ByID: %w", err)
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
	if err := stmt.QueryContext(ctx, s.reader(ctx), &rows); err != nil {
		return nil, fmt.Errorf("agent.postgres: List: %w", err)
	}
	out := make([]*Agent, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toAgent())
	}
	return out, nil
}
