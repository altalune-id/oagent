package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/schema/entity"
)

// Save upserts an agent by id, bumping updated_at.
func (s *postgresStore) Save(ctx context.Context, a *Agent) error {
	now := time.Now().UTC()
	stmt := s.agents.INSERT(s.agents.AllColumns).
		VALUES(a.ID, a.PublicID, a.OrgID, a.ProjectID, a.Slug, a.Name, a.Description, createdByExpr(a), a.CreatedAt.UTC(), now).
		ON_CONFLICT(s.agents.ID).
		DO_UPDATE(postgres.SET(
			s.agents.Slug.SET(postgres.String(a.Slug)),
			s.agents.Name.SET(postgres.String(a.Name)),
			s.agents.Description.SET(postgres.String(a.Description)),
			s.agents.UpdatedAt.SET(postgres.TimestampzT(now)),
		))
	if _, err := stmt.ExecContext(ctx, db.Writer(ctx, s.pool)); err != nil {
		if db.IsUniqueViolation(err) {
			return &AlreadyExistsError{Slug: a.Slug}
		}
		return fmt.Errorf("agent.postgres: Save: %w", err)
	}
	return nil
}

// Delete removes an agent by id.
func (s *postgresStore) Delete(ctx context.Context, id uuid.UUID) error {
	stmt := s.agents.DELETE().WHERE(s.agents.ID.EQ(postgres.UUID(id)))
	res, err := stmt.ExecContext(ctx, db.Writer(ctx, s.pool))
	if err != nil {
		return fmt.Errorf("agent.postgres: Delete: %w", err)
	}
	if n, rowsErr := res.RowsAffected(); rowsErr == nil && n == 0 {
		return &NotFoundError{ID: id.String()}
	}
	return nil
}

func createdByExpr(a *Agent) postgres.Expression {
	if a.CreatedBy.Valid {
		return postgres.UUID(a.CreatedBy.UUID)
	}
	return entity.NullUUID()
}
