package project

import (
	"context"
	"fmt"
	"time"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/go-jet/jet/v2/qrm"
	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/schema/entity"
)

// Save upserts a project by id, bumping updated_at.
func (s *postgresStore) Save(ctx context.Context, p *Project) error {
	now := time.Now().UTC()
	stmt := s.projects.INSERT(s.projects.AllColumns).
		VALUES(p.ID, p.PublicID, p.OrgID, p.Slug, p.Name, createdByExpr(p), p.CreatedAt.UTC(), now, p.Environment).
		ON_CONFLICT(s.projects.ID).
		DO_UPDATE(postgres.SET(
			s.projects.Slug.SET(postgres.String(p.Slug)),
			s.projects.Name.SET(postgres.String(p.Name)),
			s.projects.UpdatedAt.SET(postgres.TimestampzT(now)),
		))
	if err := db.Write(ctx, s.pool, func(q qrm.DB) error { _, err := stmt.ExecContext(ctx, q); return err }); err != nil {
		if db.IsUniqueViolation(err) {
			return &AlreadyExistsError{Slug: p.Slug}
		}
		return fmt.Errorf("project.postgres: Save: %w", err)
	}
	return nil
}

// SetEnvironment writes only the environment tag of the project id, so a concurrent save of other settings keeps it.
func (s *postgresStore) SetEnvironment(ctx context.Context, id uuid.UUID, env string) error {
	t := s.projects
	stmt := t.UPDATE(t.Environment, t.UpdatedAt).
		SET(postgres.String(env), postgres.TimestampzT(time.Now().UTC())).
		WHERE(t.ID.EQ(postgres.UUID(id)))
	var n int64
	err := db.Write(ctx, s.pool, func(q qrm.DB) error {
		res, err := stmt.ExecContext(ctx, q)
		if err != nil {
			return err
		}
		n, err = res.RowsAffected()
		return err
	})
	if err != nil {
		return fmt.Errorf("project.postgres: SetEnvironment: %w", err)
	}
	if n == 0 {
		return &NotFoundError{ID: id.String()}
	}
	return nil
}

func createdByExpr(p *Project) postgres.Expression {
	if p.CreatedBy.Valid {
		return postgres.UUID(p.CreatedBy.UUID)
	}
	return entity.NullUUID()
}
