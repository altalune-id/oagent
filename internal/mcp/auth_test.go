package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/authn"
	"altalune.id/oagent/internal/kernel/authz"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/kernel/tokens"
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
	scope  tenant.Context
}

func guarded(t *testing.T, o AuthOptions, s *seen) http.Handler {
	t.Helper()
	return Authenticate(o)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.called = true
		s.p = session.PrincipalFrom(r.Context())
		s.scope = tenant.Peek(r.Context())
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

// SECURITY: tools call the controlplane handlers directly, so the request carries the principal's tenant scope as /api's interceptor gives it.
func TestAuthenticate_ScopesTheRequestToTheActiveOrg(t *testing.T) {
	p := keyPrincipal()
	p.ActiveOrgID, p.ActiveProjectID = uuid.New(), uuid.New()
	var s seen
	call(guarded(t, AuthOptions{Chain: authn.Chain{&chainStub{p: p}}, KeyPrefix: "oagent_", Required: true}, &s),
		http.Header{"Authorization": {"Bearer " + goodKey}})
	assert.Equal(t, tenant.Context{OrgID: p.ActiveOrgID, ProjectID: p.ActiveProjectID}, s.scope)
}

func TestAuthenticate_BasicPutsTheOperator(t *testing.T) {
	var s seen
	h := http.Header{"Authorization": {"Basic " + base64.StdEncoding.EncodeToString([]byte("op:pw"))}}
	rec := call(guarded(t, AuthOptions{Chain: authn.Chain{&chainStub{}}, KeyPrefix: "oagent_"}, &s), h)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, s.p.Operator)
}

const metadataURL = "https://oagent.example.com/.well-known/oauth-protected-resource/mcp"

func mcpResource(t *testing.T) tokens.Resource {
	t.Helper()
	r, err := tokens.NewResource("https://oagent.example.com/mcp")
	require.NoError(t, err)
	require.Equal(t, metadataURL, r.MetadataURL)
	return r
}

type tokenStub struct {
	p   session.Principal
	err error
}

func (s tokenStub) Authenticate(_ context.Context, cred authn.Credential) (session.Principal, error) {
	if cred.Kind != authn.KindToken {
		return session.Principal{}, errors.New("not a token")
	}
	return s.p, s.err
}

func tokenPrincipal(granted, held []string) session.Principal {
	return session.Principal{Source: session.SourceToken, UserID: uuid.New(), TokenScopes: granted, Scopes: held}
}

func toolScopes(name string) []string {
	return map[string][]string{
		"agent_create":  {scope.AgentWrite},
		"agent_list":    {scope.AgentRead},
		"apikey_create": {scope.ApikeyWrite},
		"auth_whoami":   {authz.AnyAuthenticated},
	}[name]
}

func callTool(h http.Handler, bearer, tool string) *httptest.ResponseRecorder {
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"` + tool + `","arguments":{}}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+bearer)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAuthenticate_NoCredentialIsChallengedWithTheMetadata(t *testing.T) {
	var s seen
	rec := call(guarded(t, AuthOptions{Chain: authn.Chain{&chainStub{}}, KeyPrefix: "oagent_", Required: true, Resource: mcpResource(t)}, &s), nil)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Equal(t, `Bearer resource_metadata="`+metadataURL+`"`, rec.Header().Get("WWW-Authenticate"))
}

// SECURITY: RFC 6750 Section 3.1: only the error code tells "no token" from "refused"; why stays masked.
func TestAuthenticate_RefusedCredentialIsInvalidToken(t *testing.T) {
	o := AuthOptions{Chain: authn.Chain{&chainStub{}, tokenStub{err: &tokens.InvalidTokenError{Reason: "expired"}}}, KeyPrefix: "oagent_", Resource: mcpResource(t)}
	for _, bearer := range []string{"oagent_bad", "a.b.c"} {
		var s seen
		rec := call(guarded(t, o, &s), http.Header{"Authorization": {"Bearer " + bearer}})
		assert.Equal(t, http.StatusUnauthorized, rec.Code, bearer)
		assert.Equal(t, `Bearer resource_metadata="`+metadataURL+`", error="invalid_token"`, rec.Header().Get("WWW-Authenticate"), bearer)
		assert.NotContains(t, rec.Body.String(), "expired")
	}
}

// NOTE: no WWW-Authenticate, so a client keeps its token through a provider or database outage.
func TestAuthenticate_AnOutageAnswers503WithoutAChallenge(t *testing.T) {
	var s seen
	o := AuthOptions{Chain: authn.Chain{&chainStub{}, tokenStub{err: &authn.UnavailableError{Cause: errors.New("provider down")}}}, KeyPrefix: "oagent_", Resource: mcpResource(t)}
	rec := call(guarded(t, o, &s), http.Header{"Authorization": {"Bearer a.b.c"}})
	assert.False(t, s.called)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Empty(t, rec.Header().Get("WWW-Authenticate"))
	assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	var body rootmcp.ErrorPayload
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "GEN009", body.Code)
	assert.NotContains(t, rec.Body.String(), "provider down")
}

// NOTE: RFC 6750 Section 3.1 step-up: a token never granted the tool's scope gets a 403 naming it, before dispatch.
func TestAuthenticate_TokenWithoutTheToolScopeGetsAStepUp(t *testing.T) {
	var s seen
	o := AuthOptions{Chain: authn.Chain{tokenStub{p: tokenPrincipal([]string{scope.AgentRead}, []string{scope.AgentRead})}}, Resource: mcpResource(t), ToolScopes: toolScopes}
	rec := callTool(guarded(t, o, &s), "a.b.c", "agent_create")
	assert.False(t, s.called)
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Equal(t, `Bearer resource_metadata="`+metadataURL+`", error="insufficient_scope", scope="agent:write"`, rec.Header().Get("WWW-Authenticate"))
	var body rootmcp.ErrorPayload
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "GEN007", body.Code)
	assert.Equal(t, scope.AgentWrite, body.Meta["scope"])
}

func TestAuthenticate_NoStepUpWhenConsentCannotHelp(t *testing.T) {
	for name, tc := range map[string]struct {
		p    session.Principal
		tool string
	}{
		"granted, the role refuses":    {tokenPrincipal([]string{scope.AgentWrite}, nil), "agent_create"},
		"a console scope":              {tokenPrincipal(nil, nil), "apikey_create"},
		"a marker, not a scope":        {tokenPrincipal(nil, nil), "auth_whoami"},
		"an unknown tool":              {tokenPrincipal(nil, nil), "nope"},
		"a key keeps in-result denial": {keyPrincipal(), "agent_create"},
	} {
		t.Run(name, func(t *testing.T) {
			var s seen
			chain := authn.Chain{tokenStub{p: tc.p}}
			bearer := "a.b.c"
			if tc.p.Source == session.SourceAPIKey {
				chain, bearer = authn.Chain{&chainStub{p: tc.p}}, goodKey
			}
			o := AuthOptions{Chain: chain, KeyPrefix: "oagent_", Resource: mcpResource(t), ToolScopes: toolScopes}
			rec := callTool(guarded(t, o, &s), bearer, tc.tool)
			assert.Equal(t, http.StatusOK, rec.Code)
			assert.True(t, s.called, "the tool's own check answers in the result")
		})
	}
}

func TestAuthenticate_StepUpCheckRestoresTheBody(t *testing.T) {
	o := AuthOptions{Chain: authn.Chain{tokenStub{p: tokenPrincipal([]string{scope.AgentRead}, []string{scope.AgentRead})}}, Resource: mcpResource(t), ToolScopes: toolScopes}
	var got string
	h := Authenticate(o)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		got = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	rec := callTool(h, "a.b.c", "agent_list")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, got, `"name":"agent_list"`)
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
