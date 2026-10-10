package member

import (
	"context"
	"fmt"
)

// Add inserts m; a second membership for the same org and user is an *AlreadyMemberError.
func (s *postgresStore) Add(ctx context.Context, m *Membership) error {
	t := s.memberships
	stmt := t.INSERT(t.AllColumns).VALUES(m.ID, m.PublicID, m.OrgID, m.UserID, string(m.Role), m.CreatedAt.UTC())
	if _, err := stmt.ExecContext(ctx, s.writer(ctx)); err != nil {
		if IsUniqueViolation(err) {
			return &AlreadyMemberError{OrgID: m.OrgID, UserID: m.UserID}
		}
		return fmt.Errorf("member.postgres: Add: %w", err)
	}
	return nil
}
