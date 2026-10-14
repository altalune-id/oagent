package project

import (
	"context"
	"fmt"
	"time"

	"github.com/go-jet/jet/v2/postgres"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/schema/entity"
)

// Save upserts a project by id, bumping updated_at.
func (s *postgresStore) Save(ctx context.Context, p *Project) error {
	now := time.Now().UTC()
	stmt := s.projects.INSERT(s.projects.AllColumns).
		VALUES(p.ID, p.PublicID, p.OrgID, p.Slug, p.Name, createdByExpr(p), p.CreatedAt.UTC(), now).
		ON_CONFLICT(s.projects.ID).
		DO_UPDATE(postgres.SET(
			s.projects.Slug.SET(postgres.String(p.Slug)),
			s.projects.Name.SET(postgres.String(p.Name)),
			s.projects.UpdatedAt.SET(postgres.TimestampzT(now)),
		))
	if _, err := stmt.ExecContext(ctx, db.Writer(ctx, s.pool)); err != nil {
		if db.IsUniqueViolation(err) {
			return &AlreadyExistsError{Slug: p.Slug}
		}
		return fmt.Errorf("project.postgres: Save: %w", err)
	}
	return nil
}

func createdByExpr(p *Project) postgres.Expression {
	if p.CreatedBy.Valid {
		return postgres.UUID(p.CreatedBy.UUID)
	}
	return entity.NullUUID()
}
