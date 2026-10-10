package user

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/go-jet/jet/v2/qrm"
	"github.com/google/uuid"
)

// ByID loads a user by id.
func (s *postgresStore) ByID(ctx context.Context, id uuid.UUID) (*User, error) {
	return s.one(ctx, s.reader(ctx), "ByID", s.users.ID.EQ(postgres.UUID(id)))
}

// ByEmail loads a user by normalized email from the primary.
func (s *postgresStore) ByEmail(ctx context.Context, email string) (*User, error) {
	e, err := NormalizeEmail(email)
	if err != nil {
		return nil, &NotFoundError{}
	}
	return s.one(ctx, s.writer(ctx), "ByEmail", s.users.Email.EQ(postgres.String(e)))
}

func (s *postgresStore) one(ctx context.Context, conn qrm.DB, op string, where postgres.BoolExpression) (*User, error) {
	stmt := postgres.SELECT(s.users.AllColumns).FROM(s.users).WHERE(where).LIMIT(1)
	var row userRow
	if err := stmt.QueryContext(ctx, conn, &row); err != nil {
		if errors.Is(err, qrm.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return nil, &NotFoundError{}
		}
		return nil, fmt.Errorf("user.postgres: %s: %w", op, err)
	}
	return row.toUser(), nil
}
