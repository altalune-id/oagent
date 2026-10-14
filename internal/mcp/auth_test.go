package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/authn"
	"altalune.id/oagent/internal/kernel/authz"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/internal/permission"
	rootmcp "altalune.id/oagent/mcp"
	"altalune.id/oagent/scope"
)

const goodKey = "oagent_good"

type chainStub struct {
	calls int
	p     session.Principal
}

func (c *chainStub) Authenticate(_ context.Context, cred authn.Credential) (session.Principal, error) {
	c.calls++
	switch {
	case cred.Kind == authn.KindAPIKey && cred.Token == goodKey:
		return c.p, nil
	case cred.Kind == authn.KindBasic && cred.User == "op" && cred.Password == "pw":
		return session.Principal{Source: session.SourceBasic, Operator: true}, nil
	}
	return session.Principal{}, &authn.UnauthorizedError{}
}

type seen struct {
	called bool
	p      session.Principal
}

func guarded(t *testing.T, o AuthOptions, s *seen) http.Handler {
	t.Helper()
	return Authenticate(o)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.called = true
		s.p = session.PrincipalFrom(r.Context())
		w.WriteHeader(http.StatusOK)
	}))
}

func call(h http.Handler, header http.Header) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	maps.Copy(req.Header, header)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func keyPrincipal() session.Principal {
	return session.Principal{Source: session.SourceAPIKey, KeyID: uuid.New(), Scopes: []string{scope.AgentRead}}
}

func TestAuthenticate_NoCredentialOptionalPassesAnonymous(t *testing.T) {
	var s seen
	rec := call(guarded(t, AuthOptions{Chain: authn.Chain{&chainStub{}}, KeyPrefix: "oagent_"}, &s), nil)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, s.called)
	assert.Empty(t, s.p.Source)
}

func TestAuthenticate_NoCredentialRequiredIs401(t *testing.T) {
	var s seen
	rec := call(guarded(t, AuthOptions{Chain: authn.Chain{&chainStub{}}, KeyPrefix: "oagent_", Required: true}, &s), nil)
	assert.False(t, s.called)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Equal(t, "Bearer", rec.Header().Get("WWW-Authenticate"))
	assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	var body rootmcp.ErrorPayload
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "GEN002", body.Code)
	assert.Equal(t, "Sign in required", body.Message)
}

func TestAuthenticate_EveryFailureIsTheSame401(t *testing.T) {
	failures := map[string]http.Header{
		"bad key":          {"Authorization": {"Bearer oagent_bad"}},
		"malformed bearer": {"Authorization": {"Bearer not-a-key"}},
		"unknown scheme":   {"Authorization": {"Token abc"}},
		"bad basic":        {"Authorization": {"Basic " + base64.StdEncoding.EncodeToString([]byte("op:nope"))}},
		"broken basic":     {"Authorization": {"Basic %%%"}},
	}
	var bodies []string
	for name, h := range failures {
		var s seen
		rec := call(guarded(t, AuthOptions{Chain: authn.Chain{&chainStub{}}, KeyPrefix: "oagent_"}, &s), h)
		assert.False(t, s.called, name)
		assert.Equal(t, http.StatusUnauthorized, rec.Code, name)
		assert.Equal(t, "Bearer", rec.Header().Get("WWW-Authenticate"), name)
		bodies = append(bodies, rec.Body.String())
	}
	for _, b := range bodies[1:] {
		assert.Equal(t, bodies[0], b, "every failure answers the same body")
	}
}

// SECURITY: /mcp has no CSRF guard, so a browser session cookie must never authenticate it.
func TestAuthenticate_CookieIsIgnored(t *testing.T) {
	secret := []byte("0123456789abcdef0123456789abcdef")
	cookie := session.CookieName + "=" + session.Sign(secret, "sid")
	stub := &chainStub{p: session.Principal{Source: session.SourceLocal, UserID: uuid.New()}}

	var s seen
	rec := call(guarded(t, AuthOptions{Chain: authn.Chain{stub}, KeyPrefix: "oagent_"}, &s), http.Header{"Cookie": {cookie}})
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, s.p.Source, "a cookie alone is no credential")
	assert.Zero(t, stub.calls)

	rec = call(guarded(t, AuthOptions{Chain: authn.Chain{stub}, KeyPrefix: "oagent_", Required: true}, &s), http.Header{"Cookie": {cookie}})
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAuthenticate_ValidKeyPutsThePrincipal(t *testing.T) {
	p := keyPrincipal()
	var s seen
	rec := call(guarded(t, AuthOptions{Chain: authn.Chain{&chainStub{p: p}}, KeyPrefix: "oagent_", Required: true}, &s),
		http.Header{"Authorization": {"Bearer " + goodKey}})
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, p.KeyID, s.p.KeyID)
}

func TestAuthenticate_BasicPutsTheOperator(t *testing.T) {
	var s seen
	h := http.Header{"Authorization": {"Basic " + base64.StdEncoding.EncodeToString([]byte("op:pw"))}}
	rec := call(guarded(t, AuthOptions{Chain: authn.Chain{&chainStub{}}, KeyPrefix: "oagent_"}, &s), h)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, s.p.Operator)
}

func TestAuthenticate_ResourceMetadataInTheChallenge(t *testing.T) {
	var s seen
	o := AuthOptions{Chain: authn.Chain{&chainStub{}}, KeyPrefix: "oagent_", Required: true, ResourceMetadataURL: "https://a.example/.well-known/oauth-protected-resource/mcp"}
	rec := call(guarded(t, o, &s), nil)
	assert.Equal(t, `Bearer resource_metadata="https://a.example/.well-known/oauth-protected-resource/mcp"`, rec.Header().Get("WWW-Authenticate"))
}

func TestWWWAuthenticate(t *testing.T) {
	assert.Equal(t, "Bearer", WWWAuthenticate(""))
	assert.Equal(t, `Bearer resource_metadata="https://a/x"`, WWWAuthenticate("https://a/x"))
}

func allToolScopes() []string {
	var out []string
	for _, perms := range permission.Table() {
		for _, s := range perms {
			if !slices.Contains(out, s) {
				out = append(out, s)
			}
		}
	}
	slices.Sort(out)
	return out
}

func TestScopesFromContext(t *testing.T) {
	all := allToolScopes()
	notPerson := slices.DeleteFunc(slices.Clone(all), func(s string) bool { return s == authz.PersonOnly })
	tests := []struct {
		name string
		p    *session.Principal
		want []string
	}{
		{name: "no principal holds no scope", want: nil},
		{name: "owner", p: &session.Principal{Source: session.SourceLocal, UserID: uuid.New(), Scopes: []string{"*"}}, want: all},
		{name: "system", p: new(session.System()), want: notPerson},
		{name: "basic", p: &session.Principal{Source: session.SourceBasic, Operator: true}, want: notPerson},
		{name: "org key with agent:read", p: &session.Principal{Source: session.SourceAPIKey, Scopes: []string{scope.AgentRead}},
			want: []string{authz.AnyAuthenticated, scope.AgentRead}},
		{name: "personal key keeps its intersected scopes", p: &session.Principal{Source: session.SourceAPIKey, OwnerID: uuid.New(), Scopes: []string{scope.AgentRead, scope.WebhookRead}},
			want: []string{authz.AnyAuthenticated, scope.AgentRead, scope.WebhookRead}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.p != nil {
				ctx = session.PrincipalInto(ctx, *tt.p)
			}
			assert.Equal(t, tt.want, ScopesFromContext(ctx))
		})
	}
}

func TestScopesFromContext_MemberHasNoOrgWrite(t *testing.T) {
	p := session.Principal{Source: session.SourceLocal, UserID: uuid.New(), Scopes: member.BuiltinPermissions(member.RoleMember)}
	got := ScopesFromContext(session.PrincipalInto(context.Background(), p))
	assert.NotContains(t, got, scope.OrgWrite)
	assert.NotContains(t, got, scope.ProjectWrite)
	assert.Contains(t, got, scope.AgentWrite)
	assert.Contains(t, got, authz.AnyAuthenticated)
}
