package invite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/go-jet/jet/v2/qrm"
	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/schema/entity"
)

// LockEmail takes a transaction-scoped advisory lock on orgID and email.
func (s *postgresStore) LockEmail(ctx context.Context, orgID uuid.UUID, email string) error {
	return db.LockInTx(ctx, "invite:"+orgID.String()+":"+email)
}

// Insert adds i.
func (s *postgresStore) Insert(ctx context.Context, i *Invite) error {
	t := s.invites
	stmt := t.INSERT(t.AllColumns).VALUES(
		i.ID, i.PublicID, i.OrgID, i.Email, string(i.Role), tokenExpr(i.TokenHash), i.Draft, i.EmailOnly, userExpr(i.InvitedBy),
		i.ExpiresAt.UTC(), entity.NullTimestampz(), entity.NullUUID(), entity.NullTimestampz(), i.CreatedAt.UTC(), i.UpdatedAt.UTC(),
	)
	if err := db.Write(ctx, s.pool, func(q qrm.DB) error { _, err := stmt.ExecContext(ctx, q); return err }); err != nil {
		return fmt.Errorf("invite.postgres: Insert: %w", err)
	}
	return nil
}

// Update writes i's role, token, draft and email-only flags, inviter and expiry.
func (s *postgresStore) Update(ctx context.Context, i *Invite) error {
	t := s.invites
	stmt := t.UPDATE().SET(
		t.Role.SET(postgres.String(string(i.Role))),
		t.TokenHash.SET(tokenExpr(i.TokenHash)),
		t.Draft.SET(postgres.Bool(i.Draft)),
		t.EmailOnly.SET(postgres.Bool(i.EmailOnly)),
		t.InvitedBy.SET(userExpr(i.InvitedBy)),
		t.ExpiresAt.SET(postgres.TimestampzT(i.ExpiresAt.UTC())),
		t.UpdatedAt.SET(postgres.TimestampzT(i.UpdatedAt.UTC())),
	).WHERE(t.ID.EQ(postgres.UUID(i.ID)))
	return s.execOne(ctx, "Update", stmt)
}

// MarkAccepted uses up the invite id for userID, only while it is live, unexpired and not a draft, and returns it.
func (s *postgresStore) MarkAccepted(ctx context.Context, id, userID uuid.UUID, now time.Time) (*Invite, error) {
	t := s.invites
	at := postgres.TimestampzT(now.UTC())
	stmt := t.UPDATE().SET(
		t.AcceptedAt.SET(at),
		t.AcceptedBy.SET(postgres.UUID(userID)),
		t.UpdatedAt.SET(at),
	).WHERE(t.ID.EQ(postgres.UUID(id)).
		AND(t.AcceptedAt.IS_NULL()).AND(t.RevokedAt.IS_NULL()).
		AND(t.Draft.IS_FALSE()).AND(t.ExpiresAt.GT(at)),
	).RETURNING(t.AllColumns)
	var row inviteRow
	if err := db.Write(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &row) }); err != nil {
		if db.IsNoRows(err) {
			return nil, &NotFoundError{}
		}
		return nil, fmt.Errorf("invite.postgres: MarkAccepted: %w", err)
	}
	return row.toInvite(), nil
}

// Revoke sets revoked_at on the invite id, only while it is live.
func (s *postgresStore) Revoke(ctx context.Context, id uuid.UUID, now time.Time) error {
	t := s.invites
	at := postgres.TimestampzT(now.UTC())
	stmt := t.UPDATE().SET(t.RevokedAt.SET(at), t.UpdatedAt.SET(at)).
		WHERE(t.ID.EQ(postgres.UUID(id)).AND(t.AcceptedAt.IS_NULL()).AND(t.RevokedAt.IS_NULL()))
	return s.execOne(ctx, "Revoke", stmt)
}

func (s *postgresStore) execOne(ctx context.Context, op string, stmt postgres.Statement) error {
	var res sql.Result
	err := db.Write(ctx, s.pool, func(q qrm.DB) (err error) {
		res, err = stmt.ExecContext(ctx, q)
		return err
	})
	if err != nil {
		return fmt.Errorf("invite.postgres: %s: %w", op, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("invite.postgres: %s: %w", op, err)
	}
	if n == 0 {
		return &NotFoundError{}
	}
	return nil
}

func tokenExpr(hash string) postgres.StringExpression {
	if hash == "" {
		return entity.NullText()
	}
	return postgres.String(hash)
}

func userExpr(id uuid.UUID) postgres.StringExpression {
	if id == uuid.Nil {
		return entity.NullUUID()
	}
	return postgres.UUID(id)
}
