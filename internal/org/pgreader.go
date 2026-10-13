package org

import (
	"context"
	"fmt"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
)

// ByID loads an org by id.
func (s *postgresStore) ByID(ctx context.Context, id uuid.UUID) (*Org, error) {
	stmt := postgres.SELECT(s.orgs.AllColumns).
		FROM(s.orgs).
		WHERE(s.orgs.ID.EQ(postgres.UUID(id))).
		LIMIT(1)
	var row orgRow
	if err := stmt.QueryContext(ctx, db.Reader(ctx, s.pool), &row); err != nil {
		if db.IsNoRows(err) {
			return nil, &NotFoundError{ID: id.String()}
		}
		return nil, fmt.Errorf("org.postgres: ByID: %w", err)
	}
	return row.toOrg(), nil
}

// BySlug loads an org by slug.
func (s *postgresStore) BySlug(ctx context.Context, slug string) (*Org, error) {
	stmt := postgres.SELECT(s.orgs.AllColumns).
		FROM(s.orgs).
		WHERE(s.orgs.Slug.EQ(postgres.String(slug))).
		LIMIT(1)
	var row orgRow
	if err := stmt.QueryContext(ctx, db.Reader(ctx, s.pool), &row); err != nil {
		if db.IsNoRows(err) {
			return nil, &NotFoundError{Slug: slug}
		}
		return nil, fmt.Errorf("org.postgres: BySlug: %w", err)
	}
	return row.toOrg(), nil
}

// ByPublicID loads an org by public id; an unknown id is a *NotFoundError that does not echo it.
func (s *postgresStore) ByPublicID(ctx context.Context, publicID string) (*Org, error) {
	stmt := postgres.SELECT(s.orgs.AllColumns).
		FROM(s.orgs).
		WHERE(s.orgs.PublicID.EQ(postgres.String(publicID))).
		LIMIT(1)
	var row orgRow
	if err := stmt.QueryContext(ctx, db.Reader(ctx, s.pool), &row); err != nil {
		if db.IsNoRows(err) {
			return nil, &NotFoundError{}
		}
		return nil, fmt.Errorf("org.postgres: ByPublicID: %w", err)
	}
	return row.toOrg(), nil
}

// List returns every org ordered by creation time.
func (s *postgresStore) List(ctx context.Context) ([]*Org, error) {
	stmt := postgres.SELECT(s.orgs.AllColumns).
		FROM(s.orgs).
		ORDER_BY(s.orgs.CreatedAt.ASC())
	var rows []orgRow
	if err := stmt.QueryContext(ctx, db.Reader(ctx, s.pool), &rows); err != nil {
		return nil, fmt.Errorf("org.postgres: List: %w", err)
	}
	out := make([]*Org, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toOrg())
	}
	return out, nil
}

// System loads the deployment's system org from the primary.
func (s *postgresStore) System(ctx context.Context) (*Org, error) {
	stmt := postgres.SELECT(s.orgs.AllColumns).
		FROM(s.orgs).
		WHERE(s.orgs.IsSystem.IS_TRUE()).
		LIMIT(1)
	var row orgRow
	if err := stmt.QueryContext(ctx, db.Writer(ctx, s.pool), &row); err != nil {
		if db.IsNoRows(err) {
			return nil, &NotFoundError{}
		}
		return nil, fmt.Errorf("org.postgres: System: %w", err)
	}
	return row.toOrg(), nil
}
