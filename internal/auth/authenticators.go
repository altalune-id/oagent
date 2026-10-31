package auth

import (
	"context"

	"altalune.id/oagent/internal/kernel/authn"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/kernel/tokens"
)

// SessionAuthenticator loads a cookie session and rebuilds its authority.
type SessionAuthenticator struct {
	Sessions session.Store
	Auth     *Service
}

// Authenticate implements authn.Authenticator for session credentials.
func (a SessionAuthenticator) Authenticate(ctx context.Context, c authn.Credential) (session.Principal, error) {
	if c.Kind != authn.KindSession {
		return session.Principal{}, &InvalidCredentialsError{}
	}
	p, ok, err := a.Sessions.Load(ctx, c.Token)
	if err != nil {
		return session.Principal{}, err
	}
	if !ok {
		return session.Principal{}, &InvalidCredentialsError{}
	}
	return a.Auth.Refresh(ctx, p)
}

// BasicAuthenticator checks the shared basic-mode credentials.
type BasicAuthenticator struct{ Auth *Service }

// Authenticate implements authn.Authenticator for basic credentials.
func (a BasicAuthenticator) Authenticate(ctx context.Context, c authn.Credential) (session.Principal, error) {
	if c.Kind != authn.KindBasic {
		return session.Principal{}, &InvalidCredentialsError{}
	}
	return a.Auth.Basic(ctx, c.User, c.Password)
}

// TokenAuthenticator checks an access token from the OIDC provider and resolves the person linked to it.
type TokenAuthenticator struct {
	Verifier tokens.Verifier
	Auth     *Service
}

// Authenticate implements authn.Authenticator for access tokens.
func (a TokenAuthenticator) Authenticate(ctx context.Context, c authn.Credential) (session.Principal, error) {
	if c.Kind != authn.KindToken {
		return session.Principal{}, &InvalidCredentialsError{}
	}
	p, err := a.Verifier.Verify(ctx, c.Token)
	if err != nil {
		return session.Principal{}, err
	}
	return a.Auth.Token(ctx, p)
}
