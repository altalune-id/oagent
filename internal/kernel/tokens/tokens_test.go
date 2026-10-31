package tokens_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/altalune-id/oidcrp"
	"github.com/altalune-id/oidcrp/oidcrptest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/authn"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/kernel/tokens"
)

const (
	mcpAudience   = "https://oagent.example.com/mcp"
	otherAudience = "https://other.example.com/mcp"
)

func newVerifier(t *testing.T, idp *oidcrptest.IDP) tokens.Verifier {
	t.Helper()
	v, err := tokens.NewVerifier(tokens.Config{Issuer: idp.Issuer(), Audience: mcpAudience}, oidcrp.NewKeySource(idp.Issuer(), idp.Client()))
	require.NoError(t, err)
	return v
}

func TestNewVerifier_NeedsIssuerAndAudience(t *testing.T) {
	_, err := tokens.NewVerifier(tokens.Config{Issuer: "https://issuer.example.com"}, nil)
	assert.Error(t, err)
	_, err = tokens.NewVerifier(tokens.Config{Audience: mcpAudience}, nil)
	assert.Error(t, err)
}

func TestVerify_MapsAnAccessTokenToATokenPrincipal(t *testing.T) {
	idp := oidcrptest.New(t)
	v := newVerifier(t, idp)
	raw := idp.AccessToken(map[string]any{"aud": mcpAudience, "scope": "agent:read member:write", "email": "x@example.com", "org_id": "o_other"})

	p, err := v.Verify(t.Context(), raw)
	require.NoError(t, err)
	assert.Equal(t, session.SourceToken, p.Source)
	assert.Equal(t, idp.Issuer(), p.IDPIssuer)
	assert.Equal(t, idp.User.Subject, p.IDPSubject)
	assert.Equal(t, []string{"agent:read", "member:write"}, p.Scopes)
	assert.Empty(t, p.Email, "a bearer token never supplies the profile")
	assert.Equal(t, uuid0, p.UserID, "the person is resolved from a link, never from the token")
	assert.Equal(t, uuid0, p.ActiveOrgID, "a token never names its org")
}

func TestVerify_RefusesWhatIsNotAnAccessTokenForThisResource(t *testing.T) {
	idp := oidcrptest.New(t)
	v := newVerifier(t, idp)
	now := time.Now()
	for name, raw := range map[string]string{
		"typ JWT with a valid aud": idp.IDToken(idp.AccessTokenClaims(map[string]any{"aud": mcpAudience})),
		"an ID token": idp.IDToken(map[string]any{
			"iss": idp.Issuer(), "sub": idp.User.Subject, "aud": "test-client", "iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
		}),
		"another resource": idp.AccessToken(map[string]any{"aud": otherAudience}),
		"another issuer":   idp.AccessToken(map[string]any{"aud": mcpAudience, "iss": "https://other.example.com"}),
		"expired": idp.AccessToken(map[string]any{
			"aud": mcpAudience, "iat": now.Add(-2 * time.Hour).Unix(), "exp": now.Add(-time.Hour).Unix(),
		}),
		"opaque":      "opaque-token",
		"not a token": "a.b.c",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := v.Verify(t.Context(), raw)
			assert.ErrorAs(t, err, new(*tokens.InvalidTokenError))
			assert.False(t, authn.IsUnavailableError(err))
		})
	}
}

func TestVerify_AnIssuerOutageIsNotARefusal(t *testing.T) {
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(down.Close)
	v, err := tokens.NewVerifier(tokens.Config{Issuer: down.URL, Audience: mcpAudience}, oidcrp.NewKeySource(down.URL, down.Client()))
	require.NoError(t, err)

	_, err = v.Verify(t.Context(), "aaa.bbb.ccc")
	assert.True(t, authn.IsUnavailableError(err), "err = %v", err)
	assert.NotErrorAs(t, err, new(*tokens.InvalidTokenError), "an outage never reads as a bad token")
}
