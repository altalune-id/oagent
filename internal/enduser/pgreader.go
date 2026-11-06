package enduser

import (
	"context"
	"fmt"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/go-jet/jet/v2/qrm"
	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/tenant"
)

// ByID loads an end user of the org on ctx by id.
func (s *postgresStore) ByID(ctx context.Context, id uuid.UUID) (*EndUser, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	t := s.users
	stmt := postgres.SELECT(t.AllColumns).
		FROM(t).
		WHERE(t.ID.EQ(postgres.UUID(id)).AND(t.OrgID.EQ(postgres.UUID(tc.OrgID)))).
		LIMIT(1)
	var row endUserRow
	if err := db.Read(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &row) }); err != nil {
		if db.IsNoRows(err) {
			return nil, &NotFoundError{ID: id.String()}
		}
		return nil, fmt.Errorf("enduser.postgres: ByID: %w", err)
	}
	return row.toEndUser(), nil
}

// Owners maps each of ids already known in projectID to its end user.
func (s *postgresStore) Owners(ctx context.Context, projectID uuid.UUID, ids []Identity) (map[Identity]uuid.UUID, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	t := s.identities
	match := make([]postgres.BoolExpression, 0, len(ids))
	for _, id := range ids {
		match = append(match, t.Kind.EQ(postgres.String(string(id.Kind))).AND(t.Value.EQ(postgres.String(id.Value))))
	}
	stmt := postgres.SELECT(t.Kind, t.Value, t.EndUserID).
		FROM(t).
		WHERE(t.ProjectID.EQ(postgres.UUID(projectID)).
			AND(t.OrgID.EQ(postgres.UUID(tc.OrgID))).
			AND(postgres.OR(match...)))
	var rows []identityRow
	if err := db.Read(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &rows) }); err != nil {
		return nil, fmt.Errorf("enduser.postgres: Owners: %w", err)
	}
	out := make(map[Identity]uuid.UUID, len(rows))
	for _, r := range rows {
		out[Identity{Kind: Kind(r.Kind), Value: r.Value}] = r.EndUserID
	}
	return out, nil
}
