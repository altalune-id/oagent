// Package tokens verifies the OIDC provider's access tokens for one resource server through oauthrs.
package tokens

import (
	"context"
	"errors"

	"github.com/altalune-id/oauthrs"
	"github.com/altalune-id/oidcrp"

	"altalune.id/oagent/internal/kernel/authn"
	"altalune.id/oagent/internal/kernel/session"
)

// Verifier verifies a bearer token and returns the token Principal, before the person behind it is resolved.
type Verifier interface {
	Verify(ctx context.Context, bearer string) (session.Principal, error)
}

// NewVerifier builds the Verifier for cfg over the provider's key source keys, which nil builds.
func NewVerifier(cfg Config, keys *oidcrp.KeySource) (Verifier, error) {
	if cfg.Issuer == "" || cfg.Audience == "" {
		return nil, errors.New("tokens: issuer and audience are required")
	}
	rs, err := oauthrs.New(oauthrs.Config{Issuer: cfg.Issuer, Audience: cfg.Audience, Keys: keys, ClockSkew: cfg.ClockSkew})
	if err != nil {
		return nil, err
	}
	return &verifier{rs: rs}, nil
}

type verifier struct {
	rs *oauthrs.Verifier
}

func (v *verifier) Verify(ctx context.Context, raw string) (session.Principal, error) {
	tok, err := v.rs.Verify(ctx, raw)
	if err != nil {
		return session.Principal{}, refusal(err)
	}
	return session.Principal{
		Source:     session.SourceToken,
		IDPIssuer:  tok.Issuer,
		IDPSubject: tok.Subject,
		Scopes:     tok.Scopes,
	}, nil
}

// NOTE: a failure oauthrs does not type as a token problem is a provider outage, so a client keeps its token.
func refusal(err error) error {
	switch {
	case oauthrs.IsExpiredTokenError(err):
		return &InvalidTokenError{Reason: "expired", Cause: err}
	case oauthrs.IsInvalidTokenError(err), oauthrs.IsMissingTokenError(err), oauthrs.IsMalformedRequestError(err):
		return &InvalidTokenError{Reason: "refused by the resource server", Cause: err}
	}
	return &authn.UnavailableError{Cause: err}
}
