package invite

import (
	"context"
	"fmt"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
)

// Live returns orgID's invite for email that is neither accepted nor revoked, locked FOR UPDATE.
func (s *postgresStore) Live(ctx context.Context, orgID uuid.UUID, email string) (*Invite, error) {
	if _, ok := db.CurrentTx(ctx); !ok {
		return nil, errLockOutsideTx
	}
	t := s.invites
	return s.one(ctx, "Live", postgres.SELECT(t.AllColumns).FROM(t).
		WHERE(t.OrgID.EQ(postgres.UUID(orgID)).AND(t.Email.EQ(postgres.String(email))).
			AND(t.AcceptedAt.IS_NULL()).AND(t.RevokedAt.IS_NULL())).
		FOR(postgres.UPDATE()))
}

// Lock returns the invite id, locked FOR UPDATE.
func (s *postgresStore) Lock(ctx context.Context, id uuid.UUID) (*Invite, error) {
	if _, ok := db.CurrentTx(ctx); !ok {
		return nil, errLockOutsideTx
	}
	t := s.invites
	return s.one(ctx, "Lock", postgres.SELECT(t.AllColumns).FROM(t).WHERE(t.ID.EQ(postgres.UUID(id))).FOR(postgres.UPDATE()))
}

// ByID returns the invite id.
func (s *postgresStore) ByID(ctx context.Context, id uuid.UUID) (*Invite, error) {
	t := s.invites
	return s.one(ctx, "ByID", postgres.SELECT(t.AllColumns).FROM(t).WHERE(t.ID.EQ(postgres.UUID(id))))
}

// ByTokenHash returns the invite whose token hashes to hash.
func (s *postgresStore) ByTokenHash(ctx context.Context, hash string) (*Invite, error) {
	t := s.invites
	return s.one(ctx, "ByTokenHash", postgres.SELECT(t.AllColumns).FROM(t).WHERE(t.TokenHash.EQ(postgres.String(hash))))
}

// ByPublicID returns the invite with publicID.
func (s *postgresStore) ByPublicID(ctx context.Context, publicID string) (*Invite, error) {
	t := s.invites
	return s.one(ctx, "ByPublicID", postgres.SELECT(t.AllColumns).FROM(t).WHERE(t.PublicID.EQ(postgres.String(publicID))))
}

// ListLive returns orgID's live invites and drafts with the inviter's name, or their email when the name is empty.
func (s *postgresStore) ListLive(ctx context.Context, orgID uuid.UUID) ([]*Invite, error) {
	t, u := s.invites, s.users
	stmt := postgres.SELECT(t.AllColumns, u.Name, u.Email).
		FROM(t.LEFT_JOIN(u, u.ID.EQ(t.InvitedBy))).
		WHERE(t.OrgID.EQ(postgres.UUID(orgID)).AND(t.AcceptedAt.IS_NULL()).AND(t.RevokedAt.IS_NULL())).
		ORDER_BY(t.CreatedAt.ASC(), t.Email.ASC())
	var rows []listedRow
	if err := stmt.QueryContext(ctx, db.Writer(ctx, s.pool), &rows); err != nil && !db.IsNoRows(err) {
		return nil, fmt.Errorf("invite.postgres: ListLive: %w", err)
	}
	out := make([]*Invite, 0, len(rows))
	for i := range rows {
		inv := rows[i].Invite.toInvite()
		inv.InviterName = rows[i].InviterName.String
		if inv.InviterName == "" {
			inv.InviterName = rows[i].InviterEmail.String
		}
		out = append(out, inv)
	}
	return out, nil
}

func (s *postgresStore) one(ctx context.Context, op string, stmt postgres.SelectStatement) (*Invite, error) {
	var row inviteRow
	if err := stmt.QueryContext(ctx, db.Writer(ctx, s.pool), &row); err != nil {
		if db.IsNoRows(err) {
			return nil, &NotFoundError{}
		}
		return nil, fmt.Errorf("invite.postgres: %s: %w", op, err)
	}
	return row.toInvite(), nil
}
