package user

import (
	"context"
	"fmt"
	"time"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/go-jet/jet/v2/qrm"
	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/schema/entity"
)

// Save upserts a user by id, bumping updated_at.
func (s *postgresStore) Save(ctx context.Context, u *User) error {
	now := time.Now().UTC()
	stmt := s.users.INSERT(s.users.AllColumns).
		VALUES(u.ID, u.PublicID, u.Email, u.Name, u.AvatarURL, u.PasswordHash,
			nullText(u.IDPIssuer), nullText(u.IDPSubject), u.Locale, termsExpr(u), u.CreatedAt.UTC(), now).
		ON_CONFLICT(s.users.ID).
		DO_UPDATE(postgres.SET(
			s.users.Email.SET(postgres.String(u.Email)),
			s.users.Name.SET(postgres.String(u.Name)),
			s.users.AvatarURL.SET(postgres.String(u.AvatarURL)),
			s.users.PasswordHash.SET(postgres.String(u.PasswordHash)),
			s.users.IDPIssuer.SET(nullText(u.IDPIssuer)),
			s.users.IDPSubject.SET(nullText(u.IDPSubject)),
			s.users.Locale.SET(postgres.String(u.Locale)),
			s.users.TermsAcceptedAt.SET(termsExpr(u)),
			s.users.UpdatedAt.SET(postgres.TimestampzT(now)),
		))
	if err := db.Write(ctx, s.pool, func(q qrm.DB) error { _, err := stmt.ExecContext(ctx, q); return err }); err != nil {
		if db.IsUniqueViolation(err) {
			return &AlreadyExistsError{Email: u.Email}
		}
		return fmt.Errorf("user.postgres: Save: %w", err)
	}
	u.UpdatedAt = now
	return nil
}

// AcceptTerms sets terms_accepted_at to the database time and returns the stored user.
func (s *postgresStore) AcceptTerms(ctx context.Context, id uuid.UUID) (*User, error) {
	stmt := s.users.UPDATE(s.users.TermsAcceptedAt, s.users.UpdatedAt).
		SET(postgres.NOW(), postgres.NOW()).
		WHERE(s.users.ID.EQ(postgres.UUID(id))).
		RETURNING(s.users.AllColumns)
	var row userRow
	if err := db.Write(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &row) }); err != nil {
		if db.IsNoRows(err) {
			return nil, &NotFoundError{}
		}
		return nil, fmt.Errorf("user.postgres: AcceptTerms: %w", err)
	}
	return row.toUser(), nil
}

// SECURITY: name and updated_at only, so a rename never writes back a password hash it read earlier.
func (s *postgresStore) SetName(ctx context.Context, id uuid.UUID, name string) (*User, error) {
	stmt := s.users.UPDATE(s.users.Name, s.users.UpdatedAt).
		SET(postgres.String(name), postgres.NOW()).
		WHERE(s.users.ID.EQ(postgres.UUID(id))).
		RETURNING(s.users.AllColumns)
	var row userRow
	if err := db.Write(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &row) }); err != nil {
		if db.IsNoRows(err) {
			return nil, &NotFoundError{}
		}
		return nil, fmt.Errorf("user.postgres: SetName: %w", err)
	}
	return row.toUser(), nil
}

func nullText(v string) postgres.StringExpression {
	if v == "" {
		return entity.NullText()
	}
	return postgres.String(v)
}

func termsExpr(u *User) postgres.TimestampzExpression {
	if u.TermsAcceptedAt == nil {
		return entity.NullTimestampz()
	}
	return postgres.TimestampzT(u.TermsAcceptedAt.UTC())
}

// SetPasswordHash updates only password_hash, so a stale read never writes back other columns.
func (s *postgresStore) SetPasswordHash(ctx context.Context, id uuid.UUID, hash string) error {
	stmt := s.users.UPDATE(s.users.PasswordHash, s.users.UpdatedAt).
		SET(postgres.String(hash), postgres.NOW()).
		WHERE(s.users.ID.EQ(postgres.UUID(id))).
		RETURNING(s.users.ID)
	var row struct {
		ID uuid.UUID `alias:"users.id"`
	}
	err := db.Write(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &row) })
	switch {
	case db.IsNoRows(err):
		return &NotFoundError{}
	case err != nil:
		return fmt.Errorf("user.postgres: SetPasswordHash: %w", err)
	}
	return nil
}

// LinkIDP sets the user's identity only while it has none, in one conditional UPDATE.
func (s *postgresStore) LinkIDP(ctx context.Context, id uuid.UUID, issuer, subject string) (bool, error) {
	stmt := s.users.UPDATE(s.users.IDPIssuer, s.users.IDPSubject, s.users.UpdatedAt).
		SET(postgres.String(issuer), postgres.String(subject), postgres.NOW()).
		WHERE(s.users.ID.EQ(postgres.UUID(id)).AND(s.users.IDPIssuer.IS_NULL())).
		RETURNING(s.users.ID)
	var row struct {
		ID uuid.UUID `alias:"users.id"`
	}
	err := db.Write(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &row) })
	switch {
	case db.IsNoRows(err):
		return false, nil
	case db.IsUniqueViolation(err):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("user.postgres: LinkIDP: %w", err)
	}
	return true, nil
}

// Lock takes the user's row FOR NO KEY UPDATE in the unit of work on ctx. NOTE: a foreign key's KEY SHARE doesn't wait on it.
func (s *postgresStore) Lock(ctx context.Context, id uuid.UUID) error {
	if _, ok := db.CurrentTx(ctx); !ok {
		return errLockOutsideTx
	}
	stmt := postgres.SELECT(s.users.ID).FROM(s.users).WHERE(s.users.ID.EQ(postgres.UUID(id))).FOR(postgres.NO_KEY_UPDATE())
	var row struct {
		ID uuid.UUID `alias:"users.id"`
	}
	err := db.Write(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &row) })
	switch {
	case db.IsNoRows(err):
		return &NotFoundError{}
	case err != nil:
		return fmt.Errorf("user.postgres: Lock: %w", err)
	}
	return nil
}
