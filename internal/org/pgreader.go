package org

import (
	"context"
	"fmt"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/go-jet/jet/v2/qrm"
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
	if err := db.Read(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &row) }); err != nil {
		if db.IsNoRows(err) {
			return nil, &NotFoundError{ID: id.String()}
		}
		return nil, fmt.Errorf("org.postgres: ByID: %w", err)
	}
	return row.toOrg(), nil
}

// BySlug loads an org by slug through the resolve_org_by_slug definer, before any tenant scope exists.
func (s *postgresStore) BySlug(ctx context.Context, slug string) (*Org, error) {
	orgs, err := s.scanOrgs(ctx, "BySlug", s.resolveBySlugStmt, postgres.RawArgs{"#slug": slug})
	if err != nil {
		return nil, err
	}
	if len(orgs) == 0 {
		return nil, &NotFoundError{Slug: slug}
	}
	return orgs[0], nil
}

// ByPublicID loads an org by public id through a definer; an unknown id is a *NotFoundError that does not echo it.
func (s *postgresStore) ByPublicID(ctx context.Context, publicID string) (*Org, error) {
	orgs, err := s.scanOrgs(ctx, "ByPublicID", s.resolveByPublicStmt, postgres.RawArgs{"#publicID": publicID})
	if err != nil {
		return nil, err
	}
	if len(orgs) == 0 {
		return nil, &NotFoundError{}
	}
	return orgs[0], nil
}

// List returns every org ordered by creation time.
func (s *postgresStore) List(ctx context.Context) ([]*Org, error) {
	stmt := postgres.SELECT(s.orgs.AllColumns).
		FROM(s.orgs).
		ORDER_BY(s.orgs.CreatedAt.ASC())
	var rows []orgRow
	if err := db.Read(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &rows) }); err != nil {
		return nil, fmt.Errorf("org.postgres: List: %w", err)
	}
	out := make([]*Org, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toOrg())
	}
	return out, nil
}

// System loads the deployment's system org from the primary through the resolve_system_org definer, before any tenant scope exists.
func (s *postgresStore) System(ctx context.Context) (*Org, error) {
	orgs, err := s.scanOrgsWith(ctx, db.ReadPrimary, "System", s.resolveSystemOrgStmt, nil)
	if err != nil {
		return nil, err
	}
	if len(orgs) == 0 {
		return nil, &NotFoundError{}
	}
	return orgs[0], nil
}

// IDs returns every org id, oldest first, through the list_org_ids definer, for workers that then run each org scoped.
func (s *postgresStore) IDs(ctx context.Context) ([]uuid.UUID, error) {
	var rows []struct {
		ID uuid.UUID `alias:"orgs.id"`
	}
	stmt := postgres.RawStatement(s.listIDsStmt)
	if err := db.Read(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &rows) }); err != nil {
		return nil, fmt.Errorf("org.postgres: IDs: %w", err)
	}
	out := make([]uuid.UUID, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.ID)
	}
	return out, nil
}

func (s *postgresStore) scanOrgs(ctx context.Context, op, rawQuery string, args postgres.RawArgs) ([]*Org, error) {
	return s.scanOrgsWith(ctx, db.Read, op, rawQuery, args)
}

func (s *postgresStore) scanOrgsWith(
	ctx context.Context, run func(context.Context, db.Pool, func(qrm.DB) error) error, op, rawQuery string, args postgres.RawArgs,
) ([]*Org, error) {
	var rows []orgRow
	stmt := postgres.RawStatement(rawQuery, args)
	if err := run(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &rows) }); err != nil && !db.IsNoRows(err) {
		return nil, fmt.Errorf("org.postgres: %s: %w", op, err)
	}
	out := make([]*Org, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toOrg())
	}
	return out, nil
}
