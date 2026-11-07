package enduser

import (
	"context"
	"fmt"
	"strings"

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

// IdentitiesOf lists one end user's identities, oldest first.
func (s *postgresStore) IdentitiesOf(ctx context.Context, endUserID uuid.UUID) ([]Identity, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	t := s.identities
	stmt := postgres.SELECT(t.Kind, t.Value, t.EndUserID).
		FROM(t).
		WHERE(t.EndUserID.EQ(postgres.UUID(endUserID)).AND(t.OrgID.EQ(postgres.UUID(tc.OrgID)))).
		ORDER_BY(t.CreatedAt.ASC(), t.Kind.ASC())
	var rows []identityRow
	if err := db.Read(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &rows) }); err != nil && !db.IsNoRows(err) {
		return nil, fmt.Errorf("enduser.postgres: IdentitiesOf: %w", err)
	}
	out := make([]Identity, len(rows))
	for i, r := range rows {
		out[i] = Identity{Kind: Kind(r.Kind), Value: r.Value}
	}
	return out, nil
}

// IdentitiesOfMany lists each of ids' identities in the org on ctx, oldest first.
func (s *postgresStore) IdentitiesOfMany(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID][]Identity, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID][]Identity, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	in := make([]postgres.Expression, len(ids))
	for i, id := range ids {
		in[i] = postgres.UUID(id)
	}
	t := s.identities
	stmt := postgres.SELECT(t.Kind, t.Value, t.EndUserID).
		FROM(t).
		WHERE(t.EndUserID.IN(in...).AND(t.OrgID.EQ(postgres.UUID(tc.OrgID)))).
		ORDER_BY(t.CreatedAt.ASC(), t.Kind.ASC())
	var rows []identityRow
	if err := db.Read(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &rows) }); err != nil && !db.IsNoRows(err) {
		return nil, fmt.Errorf("enduser.postgres: IdentitiesOfMany: %w", err)
	}
	for _, r := range rows {
		out[r.EndUserID] = append(out[r.EndUserID], Identity{Kind: Kind(r.Kind), Value: r.Value})
	}
	return out, nil
}

// Resolve finds an end user by public id across tenants; every later read is scoped to the org it returns.
func (s *postgresStore) Resolve(ctx context.Context, publicID string) (Ref, error) {
	var row struct {
		ID        uuid.UUID `alias:"ref.id"`
		OrgID     uuid.UUID `alias:"ref.org_id"`
		ProjectID uuid.UUID `alias:"ref.project_id"`
	}
	stmt := postgres.RawStatement(s.resolve, postgres.RawArgs{"#publicID": publicID})
	if err := db.Read(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &row) }); err != nil {
		if db.IsNoRows(err) {
			return Ref{}, &NotFoundError{ID: publicID}
		}
		return Ref{}, fmt.Errorf("enduser.postgres: Resolve: %w", err)
	}
	return Ref(row), nil
}

// List lists a project's end users newest first after the cursor; query matches part of a display name or a whole identity value.
func (s *postgresStore) List(ctx context.Context, projectID uuid.UUID, query string, after Cursor, limit int) ([]*EndUser, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	stmt := s.listStmt(tc.OrgID, projectID, query, after, limit)
	var rows []endUserRow
	if err := db.Read(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &rows) }); err != nil && !db.IsNoRows(err) {
		return nil, fmt.Errorf("enduser.postgres: List: %w", err)
	}
	out := make([]*EndUser, len(rows))
	for i := range rows {
		out[i] = rows[i].toEndUser()
	}
	return out, nil
}

// SECURITY: the query travels as a bound parameter, never in the statement text a database span records.
func (s *postgresStore) listStmt(orgID, projectID uuid.UUID, query string, after Cursor, limit int) postgres.SelectStatement {
	t, i := s.users, s.identities
	where := t.ProjectID.EQ(postgres.UUID(projectID)).AND(t.OrgID.EQ(postgres.UUID(orgID)))
	if query != "" {
		byValue := postgres.SELECT(i.EndUserID).FROM(i).
			WHERE(i.ProjectID.EQ(postgres.UUID(projectID)).AND(i.OrgID.EQ(postgres.UUID(orgID))).AND(i.Value.EQ(postgres.String(query))))
		name := postgres.LOWER(t.DisplayName).LIKE(postgres.LOWER(postgres.String("%" + likeEscape(query) + "%")))
		where = where.AND(name.OR(t.ID.IN(byValue)))
	}
	if !after.At.IsZero() {
		where = where.AND(postgres.ROW(t.CreatedAt, t.PublicID).LT(postgres.ROW(postgres.TimestampzT(after.At), postgres.String(after.PublicID))))
	}
	return postgres.SELECT(t.AllColumns).FROM(t).WHERE(where).ORDER_BY(t.CreatedAt.DESC(), t.PublicID.DESC()).LIMIT(int64(max(limit, 1)))
}

func likeEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(s)
}
