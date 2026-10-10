package org

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/go-jet/jet/v2/qrm"
	"github.com/google/uuid"
)

// ByID loads an org by id.
func (s *postgresStore) ByID(ctx context.Context, id uuid.UUID) (*Org, error) {
	stmt := postgres.SELECT(s.orgs.AllColumns).
		FROM(s.orgs).
		WHERE(s.orgs.ID.EQ(postgres.UUID(id))).
		LIMIT(1)
	var row orgRow
	if err := stmt.QueryContext(ctx, s.reader(ctx), &row); err != nil {
		if errors.Is(err, qrm.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
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
	if err := stmt.QueryContext(ctx, s.reader(ctx), &row); err != nil {
		if errors.Is(err, qrm.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return nil, &NotFoundError{Slug: slug}
		}
		return nil, fmt.Errorf("org.postgres: BySlug: %w", err)
	}
	return row.toOrg(), nil
}

// List returns every org ordered by creation time.
func (s *postgresStore) List(ctx context.Context) ([]*Org, error) {
	stmt := postgres.SELECT(s.orgs.AllColumns).
		FROM(s.orgs).
		ORDER_BY(s.orgs.CreatedAt.ASC())
	var rows []orgRow
	if err := stmt.QueryContext(ctx, s.reader(ctx), &rows); err != nil {
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
	if err := stmt.QueryContext(ctx, s.writer(ctx), &row); err != nil {
		if errors.Is(err, qrm.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return nil, &NotFoundError{}
		}
		return nil, fmt.Errorf("org.postgres: System: %w", err)
	}
	return row.toOrg(), nil
}
