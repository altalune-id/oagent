package auth

import (
	"context"

	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/user"
)

// OIDCLogin is a sign-in at the OIDC provider whose ID token the relying party already validated.
type OIDCLogin struct {
	Subject       string
	Email         string
	EmailVerified bool
	Name          string
	IDToken       string
}

// LoginOIDC turns an OIDC sign-in into a Principal: the linked account, else one email links once, else a new account in cloud.
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
	if !s.deps.OpenSignup {
		return nil, &NoConsoleAccountError{Reason: NoConsoleNotInvited}
	}
	u, err := s.deps.Identities.CreateLinked(ctx, in.Email, in.Name, s.deps.Issuer, in.Subject)
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
		return u, nil
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
	return u, nil
}
