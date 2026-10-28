package boot

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/altalune-id/othra/oidcrp"
	"github.com/altalune-id/othra/oidcrp/oidcrptest"
	"github.com/go-jose/go-jose/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/config"
)

func signedBy(t *testing.T, kid string) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: jose.JSONWebKey{Key: key, KeyID: kid}}, nil)
	require.NoError(t, err)
	jws, err := signer.Sign([]byte(`{"sub":"s"}`))
	require.NoError(t, err)
	raw, err := jws.CompactSerialize()
	require.NoError(t, err)
	return raw
}

// RP-Initiated Logout §2: a hint whose key the provider still publishes is kept; a retired, forged or garbled one is left out.
func TestLiveHint(t *testing.T) {
	idp := oidcrptest.New(t)
	keys := oidcrp.NewKeySource(idp.Issuer(), idp.Client())
	live := idp.IDToken(nil)
	assert.Equal(t, live, liveHint(t.Context(), keys, live))

	idp.RotateKey()
	assert.Equal(t, live, liveHint(t.Context(), keys, live), "an older key still published verifies")
	assert.Empty(t, liveHint(t.Context(), keys, signedBy(t, "retired")), "a kid no longer published")
	assert.Empty(t, liveHint(t.Context(), keys, signedBy(t, "k1")), "a published kid with another key's signature")
	assert.Empty(t, liveHint(t.Context(), keys, "not.a.jws"))
	assert.Empty(t, liveHint(t.Context(), oidcrp.NewKeySource("https://127.0.0.1:1", idp.Client()), live), "no discovery")
}

func TestBuildOIDCProvider(t *testing.T) {
	assert.Nil(t, buildOIDCProvider(config.Config{}), "no issuer, no provider")

	idp := oidcrptest.New(t)
	cfg := config.Config{HTTP: config.HTTPConfig{BaseURL: "https://oagent.example.com/"}, OIDC: config.OIDCConfig{
		Issuer: idp.Issuer(), ClientID: "oagent", Scopes: []string{"openid", "email"},
	}}
	require.NotNil(t, buildOIDCProvider(cfg))
	p := newOIDCProvider(cfg, idp.Client())
	req, err := oidcrp.NewAuthRequest("", "")
	require.NoError(t, err)
	target, err := p.AuthCodeURL(t.Context(), req)
	require.NoError(t, err)
	assert.Contains(t, target, "redirect_uri=https%3A%2F%2Foagent.example.com%2Foauth%2Fcallback")
	assert.Contains(t, target, "scope=openid+email")
	end, err := p.EndSessionURL(t.Context(), idp.IDToken(nil), "https://oagent.example.com/login", "")
	require.NoError(t, err)
	assert.Empty(t, end, "a provider with no end_session_endpoint")
}

// NOTE: some providers name themselves with a trailing "/"; slashIssuer serves idp under such an issuer.
func slashIssuer(t *testing.T, idp *oidcrptest.IDP) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp, err := idp.Client().Get(idp.Issuer() + r.URL.Path)
		if r.Method == http.MethodPost {
			require.NoError(t, r.ParseForm())
			resp, err = idp.Client().PostForm(idp.Issuer()+r.URL.Path, r.PostForm)
		}
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		if r.URL.Path == "/.well-known/openid-configuration" {
			body = []byte(strings.ReplaceAll(string(body), idp.Issuer(), srv.URL))
			var meta map[string]any
			require.NoError(t, json.Unmarshal(body, &meta))
			meta["issuer"] = srv.URL + "/"
			body, err = json.Marshal(meta)
			require.NoError(t, err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	idp.Claims = map[string]any{"iss": srv.URL + "/"}
	return srv
}

// An issuer that ends in "/" reaches oidcrp exactly as configured: discovery and the ID token name it with the slash.
func TestOIDCProvider_IssuerWithTrailingSlash(t *testing.T) {
	idp := oidcrptest.New(t)
	idp.User = oidcrptest.User{Subject: "sub-1", Email: "ada@example.com", EmailVerified: true}
	srv := slashIssuer(t, idp)
	cfg := config.Config{HTTP: config.HTTPConfig{BaseURL: "https://oagent.example.com"}, OIDC: config.OIDCConfig{
		Issuer: srv.URL + "/", ClientID: "oagent", Scopes: []string{"openid", "email"},
	}}
	p := newOIDCProvider(cfg, srv.Client())
	req, err := oidcrp.NewAuthRequest("", "")
	require.NoError(t, err)
	target, err := p.AuthCodeURL(t.Context(), req)
	require.NoError(t, err)
	cb := idp.Authorize(target)
	cb.Set("iss", srv.URL+"/")
	tok, err := p.Exchange(t.Context(), oidcrp.ParseCallback(cb), req)
	require.NoError(t, err)
	assert.Equal(t, "sub-1", tok.Subject)
	assert.Equal(t, srv.URL, cfg.OIDC.NormalizedIssuer(), "account links keep the issuer without its slash")
}
