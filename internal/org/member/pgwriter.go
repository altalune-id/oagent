package member

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
)

var errLockOutsideTx = errors.New("member.postgres: a row lock needs a transaction")

// Add inserts m; a second membership for the same org and user is an *AlreadyMemberError.
func (s *postgresStore) Add(ctx context.Context, m *Membership) error {
	t := s.memberships
	stmt := t.INSERT(t.AllColumns).VALUES(m.ID, m.PublicID, m.OrgID, m.UserID, string(m.Role), m.CreatedAt.UTC())
	if _, err := stmt.ExecContext(ctx, db.Writer(ctx, s.pool)); err != nil {
		if db.IsUniqueViolation(err) {
			return &AlreadyMemberError{OrgID: m.OrgID, UserID: m.UserID}
		}
		return fmt.Errorf("member.postgres: Add: %w", err)
	}
	return nil
}

// AddIfAbsent inserts m, doing nothing when the org already has the user; it reports whether a row was added.
func (s *postgresStore) AddIfAbsent(ctx context.Context, m *Membership) (bool, error) {
	t := s.memberships
	stmt := t.INSERT(t.AllColumns).VALUES(m.ID, m.PublicID, m.OrgID, m.UserID, string(m.Role), m.CreatedAt.UTC()).
		ON_CONFLICT(t.OrgID, t.UserID).DO_NOTHING()
	res, err := stmt.ExecContext(ctx, db.Writer(ctx, s.pool))
	if err != nil {
		return false, fmt.Errorf("member.postgres: AddIfAbsent: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("member.postgres: AddIfAbsent: %w", err)
	}
	return n == 1, nil
}

// LockOwners locks orgID's owner rows FOR UPDATE, in user id order, and returns their user ids.
func (s *postgresStore) LockOwners(ctx context.Context, orgID uuid.UUID) ([]uuid.UUID, error) {
	tx, ok := db.CurrentTx(ctx)
	if !ok {
		return nil, errLockOutsideTx
	}
	t := s.memberships
	stmt := postgres.SELECT(t.UserID).FROM(t).
		WHERE(t.OrgID.EQ(postgres.UUID(orgID)).AND(t.Role.EQ(postgres.String(string(RoleOwner))))).
		ORDER_BY(t.UserID.ASC()).
		FOR(postgres.UPDATE())
	var rows []struct {
		UserID uuid.UUID `alias:"memberships.user_id"`
	}
	if err := stmt.QueryContext(ctx, tx, &rows); err != nil && !db.IsNoRows(err) {
		return nil, fmt.Errorf("member.postgres: LockOwners: %w", err)
	}
	out := make([]uuid.UUID, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.UserID)
	}
	return out, nil
}

// LockMember returns userID's membership in orgID, locked FOR SHARE; a missing membership is a *NotFoundError.
func (s *postgresStore) LockMember(ctx context.Context, orgID, userID uuid.UUID) (*Membership, error) {
	tx, ok := db.CurrentTx(ctx)
	if !ok {
		return nil, errLockOutsideTx
	}
	t := s.memberships
	stmt := postgres.SELECT(t.AllColumns).FROM(t).
		WHERE(t.OrgID.EQ(postgres.UUID(orgID)).AND(t.UserID.EQ(postgres.UUID(userID)))).
		FOR(postgres.SHARE())
	var row membershipRow
	if err := stmt.QueryContext(ctx, tx, &row); err != nil {
		if db.IsNoRows(err) {
			return nil, &NotFoundError{}
		}
		return nil, fmt.Errorf("member.postgres: LockMember: %w", err)
	}
	return row.toMembership(), nil
}

// UpdateRole sets userID's role in orgID; a missing membership is a *NotFoundError.
func (s *postgresStore) UpdateRole(ctx context.Context, orgID, userID uuid.UUID, role Role) error {
	t := s.memberships
	stmt := t.UPDATE(t.Role).SET(postgres.String(string(role))).
		WHERE(t.OrgID.EQ(postgres.UUID(orgID)).AND(t.UserID.EQ(postgres.UUID(userID))))
	return s.execOne(ctx, "UpdateRole", stmt)
}

// Delete removes userID from orgID; a missing membership is a *NotFoundError.
func (s *postgresStore) Delete(ctx context.Context, orgID, userID uuid.UUID) error {
	t := s.memberships
	stmt := t.DELETE().WHERE(t.OrgID.EQ(postgres.UUID(orgID)).AND(t.UserID.EQ(postgres.UUID(userID))))
	return s.execOne(ctx, "Delete", stmt)
}

func (s *postgresStore) execOne(ctx context.Context, op string, stmt postgres.Statement) error {
	res, err := stmt.ExecContext(ctx, db.Writer(ctx, s.pool))
	if err != nil {
		return fmt.Errorf("member.postgres: %s: %w", op, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("member.postgres: %s: %w", op, err)
	}
	if n == 0 {
		return &NotFoundError{}
	}
	return nil
}
