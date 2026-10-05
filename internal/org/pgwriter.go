package org

import (
	"context"
	"fmt"
	"time"

	"github.com/go-jet/jet/v2/postgres"

	"altalune.id/auth-server/schema/entity"
)

// Save upserts an org by id, bumping updated_at.
func (s *postgresStore) Save(ctx context.Context, o *Org) error {
	now := time.Now().UTC()
	stmt := s.orgs.INSERT(s.orgs.AllColumns).
		VALUES(o.ID, o.PublicID, o.Slug, o.Name, createdByExpr(o), o.CreatedAt.UTC(), now).
		ON_CONFLICT(s.orgs.ID).
		DO_UPDATE(postgres.SET(
			s.orgs.PublicID.SET(postgres.String(o.PublicID)),
			s.orgs.Slug.SET(postgres.String(o.Slug)),
			s.orgs.Name.SET(postgres.String(o.Name)),
			s.orgs.UpdatedAt.SET(postgres.TimestampzT(now)),
		))
	if _, err := stmt.ExecContext(ctx, s.writer(ctx)); err != nil {
		if IsUniqueViolation(err) {
			return &AlreadyExistsError{Slug: o.Slug}
		}
		return fmt.Errorf("org.postgres: Save: %w", err)
	}
	return nil
}

func createdByExpr(o *Org) postgres.Expression {
	if o.CreatedBy.Valid {
		return postgres.UUID(o.CreatedBy.UUID)
	}
	return entity.NullUUID()
}
