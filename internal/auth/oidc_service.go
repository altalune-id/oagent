package auth

import (
	"context"
	"strings"

	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/user"
)

// OIDCLogin is a sign-in at the OIDC provider whose ID token the relying party already validated.
type OIDCLogin struct {
	Subject       string
	Email         string
	EmailVerified bool
	Name          string
	IDToken       string
	// Invite is the public id of the invite the sign-in started from, or empty.
	Invite string
}

// LoginOIDC turns an OIDC sign-in into a Principal: the linked account, else one email links once, else an invite or cloud signup.
func (s *Service) LoginOIDC(ctx context.Context, in OIDCLogin) (session.Principal, error) {
	switch {
	case in.Subject == "":
		return session.Principal{}, &NoConsoleAccountError{Reason: NoConsoleNotInvited}
	case !in.EmailVerified:
		return session.Principal{}, &NoConsoleAccountError{Reason: NoConsoleUnverified}
	}
	email, err := user.NormalizeEmail(in.Email)
	if err != nil {
		return session.Principal{}, &NoConsoleAccountError{Reason: NoConsoleNotInvited}
	}
	in.Email = email
	u, err := s.oidcUser(ctx, in)
	if err != nil {
		return session.Principal{}, err
	}
	if !s.deps.OpenSignup {
		ms, err := s.deps.Members.ListByUser(ctx, u.ID)
		if err != nil {
			return session.Principal{}, err
		}
		if len(ms) == 0 {
			return session.Principal{}, &NoConsoleAccountError{Reason: NoConsoleRemoved}
		}
	}
	p := principalOf(u, s.now())
	p.Source, p.IDPIssuer, p.IDPSubject, p.IDToken = session.SourceOIDC, s.deps.Issuer, in.Subject, in.IDToken
	return s.Refresh(ctx, p)
}

// SECURITY: an email links only an unlinked account, once; a link is never re-pointed, so an account can't be taken over.
func (s *Service) oidcUser(ctx context.Context, in OIDCLogin) (*user.User, error) {
	if u, err := s.existing(ctx, in); err != nil || u != nil {
		return u, err
	}
	seat, err := s.seat(ctx, in)
	if err != nil {
		return nil, err
	}
	if seat == nil && !s.deps.OpenSignup {
		return nil, &NoConsoleAccountError{Reason: NoConsoleNotInvited}
	}
	u, err := s.join(ctx, in, seat)
	if !user.IsAlreadyExistsError(err) {
		return u, err
	}
	if u, err = s.existing(ctx, in); err != nil || u != nil {
		return u, err
	}
	return nil, &IdentityConflictError{}
}

// NOTE: the account linked to this sub, else the unlinked one with this email, now linked; nil when neither exists.
func (s *Service) existing(ctx context.Context, in OIDCLogin) (*user.User, error) {
	ids := s.deps.Identities
	u, err := ids.ByIDP(ctx, s.deps.Issuer, in.Subject)
	switch {
	case err == nil:
		return u, s.claim(ctx, in, u)
	case !user.IsNotFoundError(err):
		return nil, err
	}
	u, err = s.deps.Users.ByEmail(ctx, in.Email)
	switch {
	case user.IsNotFoundError(err):
		return nil, nil
	case err != nil:
		return nil, err
	}
	linked, err := ids.LinkIDP(ctx, u.ID, s.deps.Issuer, in.Subject)
	if err != nil {
		return nil, err
	}
	if !linked {
		return nil, &IdentityConflictError{}
	}
	return u, s.claim(ctx, in, u)
}

// InviteMismatch reports whether the open invite with public id invite is for another email than email.
func (s *Service) InviteMismatch(ctx context.Context, invite, email string) (bool, error) {
	seat, err := s.deps.Invites.Seat(ctx, invite)
	if err != nil || seat == nil {
		return false, err
	}
	return !strings.EqualFold(seat.Email, email), nil
}

// NOTE: only the invite the sign-in started from counts, and only when it is for this very email.
func (s *Service) seat(ctx context.Context, in OIDCLogin) (*Seat, error) {
	if in.Invite == "" {
		return nil, nil
	}
	seat, err := s.deps.Invites.Seat(ctx, in.Invite)
	if err != nil || seat == nil || !strings.EqualFold(seat.Email, in.Email) {
		return nil, err
	}
	return seat, nil
}

func (s *Service) join(ctx context.Context, in OIDCLogin, seat *Seat) (*user.User, error) {
	if seat == nil {
		return s.deps.Identities.CreateLinked(ctx, in.Email, in.Name, s.deps.Issuer, in.Subject)
	}
	var u *user.User
	err := s.deps.Transactor.RunInTx(tenant.Into(ctx, tenant.Context{OrgID: seat.OrgID}), func(ctx context.Context) error {
		var err error
		if u, err = s.deps.Identities.CreateLinked(ctx, in.Email, in.Name, s.deps.Issuer, in.Subject); err != nil {
			return err
		}
		claimed, err := s.deps.Invites.Claim(ctx, seat, u.ID)
		if err == nil && !claimed {
			err = &NoConsoleAccountError{Reason: NoConsoleNotInvited}
		}
		return err
	})
	return u, err
}

// NOTE: an existing account that started from an invite joins that org too; a used, expired or someone else's invite is skipped.
func (s *Service) claim(ctx context.Context, in OIDCLogin, u *user.User) error {
	seat, err := s.seat(ctx, in)
	if err != nil || seat == nil {
		return err
	}
	_, err = s.deps.Invites.Claim(ctx, seat, u.ID)
	return err
}
