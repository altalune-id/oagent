package member

import (
	"context"
	"fmt"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
)

// Find returns the membership of userID in orgID.
func (s *postgresStore) Find(ctx context.Context, orgID, userID uuid.UUID) (*Membership, error) {
	t := s.memberships
	stmt := postgres.SELECT(t.AllColumns).FROM(t).
		WHERE(t.OrgID.EQ(postgres.UUID(orgID)).AND(t.UserID.EQ(postgres.UUID(userID)))).
		LIMIT(1)
	var row membershipRow
	if err := stmt.QueryContext(ctx, db.Writer(ctx, s.pool), &row); err != nil {
		if db.IsNoRows(err) {
			return nil, &NotFoundError{}
		}
		return nil, fmt.Errorf("member.postgres: Find: %w", err)
	}
	return row.toMembership(), nil
}

// ListByUser returns userID's memberships, oldest first.
func (s *postgresStore) ListByUser(ctx context.Context, userID uuid.UUID) ([]*Membership, error) {
	t := s.memberships
	stmt := postgres.SELECT(t.AllColumns).FROM(t).
		WHERE(t.UserID.EQ(postgres.UUID(userID))).
		ORDER_BY(t.CreatedAt.ASC(), t.ID.ASC())
	var rows []membershipRow
	if err := stmt.QueryContext(ctx, db.Writer(ctx, s.pool), &rows); err != nil && !db.IsNoRows(err) {
		return nil, fmt.Errorf("member.postgres: ListByUser: %w", err)
	}
	out := make([]*Membership, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toMembership())
	}
	return out, nil
}

// CountOwners returns how many owners orgID has.
func (s *postgresStore) CountOwners(ctx context.Context, orgID uuid.UUID) (int, error) {
	t := s.memberships
	stmt := postgres.SELECT(postgres.COUNT(postgres.STAR).AS("count")).FROM(t).
		WHERE(t.OrgID.EQ(postgres.UUID(orgID)).AND(t.Role.EQ(postgres.String(string(RoleOwner)))))
	var res struct {
		Count int64 `alias:"count"`
	}
	if err := stmt.QueryContext(ctx, db.Writer(ctx, s.pool), &res); err != nil {
		return 0, fmt.Errorf("member.postgres: CountOwners: %w", err)
	}
	return int(res.Count), nil
}

// ListByOrg returns orgID's members with their email and name, owners first, then admins, then members, each by email.
func (s *postgresStore) ListByOrg(ctx context.Context, orgID uuid.UUID) ([]Member, error) {
	t, u := s.memberships, s.users
	stmt := postgres.SELECT(t.AllColumns, u.PublicID, u.Email, u.Name).
		FROM(t.INNER_JOIN(u, u.ID.EQ(t.UserID))).
		WHERE(t.OrgID.EQ(postgres.UUID(orgID))).
		ORDER_BY(
			t.Role.EQ(postgres.String(string(RoleOwner))).DESC(),
			t.Role.EQ(postgres.String(string(RoleAdmin))).DESC(),
			u.Email.ASC(), t.ID.ASC(),
		)
	var rows []memberRow
	if err := stmt.QueryContext(ctx, db.Writer(ctx, s.pool), &rows); err != nil && !db.IsNoRows(err) {
		return nil, fmt.Errorf("member.postgres: ListByOrg: %w", err)
	}
	out := make([]Member, 0, len(rows))
	for i := range rows {
		out = append(out, Member{
			Membership: *rows[i].Membership.toMembership(), UserPublicID: rows[i].UserPublicID, Email: rows[i].Email, Name: rows[i].Name,
		})
	}
	return out, nil
}
