package project

import (
	"context"
	"fmt"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/go-jet/jet/v2/qrm"
	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
)

// ByID loads a project by id.
func (s *postgresStore) ByID(ctx context.Context, id uuid.UUID) (*Project, error) {
	stmt := postgres.SELECT(s.projects.AllColumns).
		FROM(s.projects).
		WHERE(s.projects.ID.EQ(postgres.UUID(id))).
		LIMIT(1)
	var row projectRow
	if err := db.Read(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &row) }); err != nil {
		if db.IsNoRows(err) {
			return nil, &NotFoundError{ID: id.String()}
		}
		return nil, fmt.Errorf("project.postgres: ByID: %w", err)
	}
	return row.toProject(), nil
}

// BySlug loads a project by slug within an organization.
func (s *postgresStore) BySlug(ctx context.Context, orgID uuid.UUID, slug string) (*Project, error) {
	stmt := postgres.SELECT(s.projects.AllColumns).
		FROM(s.projects).
		WHERE(s.projects.OrgID.EQ(postgres.UUID(orgID)).AND(s.projects.Slug.EQ(postgres.String(slug)))).
		LIMIT(1)
	var row projectRow
	if err := db.Read(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &row) }); err != nil {
		if db.IsNoRows(err) {
			return nil, &NotFoundError{Slug: slug}
		}
		return nil, fmt.Errorf("project.postgres: BySlug: %w", err)
	}
	return row.toProject(), nil
}

// List returns an organization's projects ordered by creation time.
func (s *postgresStore) List(ctx context.Context, orgID uuid.UUID) ([]*Project, error) {
	stmt := postgres.SELECT(s.projects.AllColumns).
		FROM(s.projects).
		WHERE(s.projects.OrgID.EQ(postgres.UUID(orgID))).
		ORDER_BY(s.projects.CreatedAt.ASC())
	var rows []projectRow
	if err := db.Read(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &rows) }); err != nil {
		return nil, fmt.Errorf("project.postgres: List: %w", err)
	}
	out := make([]*Project, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toProject())
	}
	return out, nil
}

// Resolve names publicID's project and its org through a definer, so a caller with no active org can check reach and then scope.
func (s *postgresStore) Resolve(ctx context.Context, publicID string) (Ref, error) {
	var row struct {
		ID    uuid.UUID `alias:"projects.id"`
		OrgID uuid.UUID `alias:"projects.org_id"`
	}
	stmt := postgres.RawStatement(s.resolve, postgres.RawArgs{"#publicID": publicID})
	if err := db.Read(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &row) }); err != nil {
		if db.IsNoRows(err) {
			return Ref{}, &NotFoundError{}
		}
		return Ref{}, fmt.Errorf("project.postgres: Resolve: %w", err)
	}
	return Ref{ID: row.ID, OrgID: row.OrgID}, nil
}
