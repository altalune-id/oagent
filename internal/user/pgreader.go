package user

import (
	"context"
	"fmt"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/go-jet/jet/v2/qrm"
	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
)

// ByID loads a user by id.
func (s *postgresStore) ByID(ctx context.Context, id uuid.UUID) (*User, error) {
	return s.one(ctx, db.Reader(ctx, s.pool), "ByID", s.users.ID.EQ(postgres.UUID(id)))
}

// ByEmail loads a user by normalized email from the primary.
func (s *postgresStore) ByEmail(ctx context.Context, email string) (*User, error) {
	e, err := NormalizeEmail(email)
	if err != nil {
		return nil, &NotFoundError{}
	}
	return s.one(ctx, db.Writer(ctx, s.pool), "ByEmail", s.users.Email.EQ(postgres.String(e)))
}

// ByPublicID loads a user by public id.
func (s *postgresStore) ByPublicID(ctx context.Context, publicID string) (*User, error) {
	return s.one(ctx, db.Reader(ctx, s.pool), "ByPublicID", s.users.PublicID.EQ(postgres.String(publicID)))
}

func (s *postgresStore) one(ctx context.Context, conn qrm.DB, op string, where postgres.BoolExpression) (*User, error) {
	stmt := postgres.SELECT(s.users.AllColumns).FROM(s.users).WHERE(where).LIMIT(1)
	var row userRow
	if err := stmt.QueryContext(ctx, conn, &row); err != nil {
		if db.IsNoRows(err) {
			return nil, &NotFoundError{}
		}
		return nil, fmt.Errorf("user.postgres: %s: %w", op, err)
	}
	return row.toUser(), nil
}
