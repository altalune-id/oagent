package user

import (
	"context"
	"fmt"
	"time"

	"github.com/go-jet/jet/v2/postgres"

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
	if _, err := stmt.ExecContext(ctx, s.writer(ctx)); err != nil {
		if IsUniqueViolation(err) {
			return &AlreadyExistsError{Email: u.Email}
		}
		return fmt.Errorf("user.postgres: Save: %w", err)
	}
	u.UpdatedAt = now
	return nil
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
