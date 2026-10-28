package boot

import (
	"context"
	"net/http"
	"time"

	"github.com/altalune-id/othra/oidcrp"
	"github.com/go-jose/go-jose/v4"

	"altalune.id/oagent/config"
	"altalune.id/oagent/httpclient"
	"altalune.id/oagent/internal/auth"
)

const (
	oidcTimeout  = 10 * time.Second
	oidcCallback = "/oauth/callback"
)

type oidcProvider struct{ rp *oidcrp.Client }

// NOTE: nil without oidc.issuer; private hosts are allowed because the issuer is operator config, like a webhook URL is.
func buildOIDCProvider(cfg config.Config) auth.OIDCProvider {
	if !cfg.OIDC.Enabled() {
		return nil
	}
	return newOIDCProvider(cfg, httpclient.New(httpclient.WithAllowPrivateHosts(true), httpclient.WithTimeout(oidcTimeout)))
}

func newOIDCProvider(cfg config.Config, client *http.Client) *oidcProvider {
	return &oidcProvider{rp: oidcrp.New(oidcrp.Config{
		Issuer:       cfg.OIDC.Issuer,
		ClientID:     cfg.OIDC.ClientID,
		ClientSecret: cfg.OIDC.ClientSecret,
		RedirectURI:  publicBase(cfg.HTTP) + oidcCallback,
		Scopes:       cfg.OIDC.Scopes,
		HTTPClient:   client,
	})}
}

// AuthCodeURL implements auth.OIDCProvider.
func (p *oidcProvider) AuthCodeURL(ctx context.Context, req oidcrp.AuthRequest) (string, error) {
	return p.rp.AuthCodeURL(ctx, req)
}

// Exchange implements auth.OIDCProvider.
func (p *oidcProvider) Exchange(ctx context.Context, cb oidcrp.Callback, req oidcrp.AuthRequest) (*oidcrp.IDToken, error) {
	return p.rp.Exchange(ctx, cb, req)
}

// EndSessionURL implements auth.OIDCProvider; empty when the provider advertises no end_session_endpoint.
func (p *oidcProvider) EndSessionURL(ctx context.Context, idTokenHint, postLogoutRedirectURI, state string) (string, error) {
	return p.rp.EndSessionURL(ctx, liveHint(ctx, p.rp.Keys(), idTokenHint), postLogoutRedirectURI, state)
}

// NOTE: RP-Initiated Logout §2: a hint signed by a key the provider no longer publishes is left out, so client_id alone ends the session.
func liveHint(ctx context.Context, src *oidcrp.KeySource, hint string) string {
	meta, err := src.Metadata(ctx)
	if err != nil {
		return ""
	}
	jws, err := jose.ParseSigned(hint, meta.SigningAlgorithms())
	if err != nil || len(jws.Signatures) != 1 {
		return ""
	}
	key, err := src.Key(ctx, jws.Signatures[0].Header.KeyID)
	if err != nil {
		return ""
	}
	if _, err := jws.Verify(key); err != nil {
		return ""
	}
	return hint
}
