//go:build integration

package boot

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/altalune-id/oidcrp/oidcrptest"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/config"
	authv1 "altalune.id/oagent/gen/go/auth/v1"
	"altalune.id/oagent/gen/go/auth/v1/authv1connect"
	invitev1 "altalune.id/oagent/gen/go/invite/v1"
	"altalune.id/oagent/gen/go/invite/v1/invitev1connect"
	"altalune.id/oagent/internal/apikey"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/internal/user"
)

const mcpOAuthAudience = "https://oagent.example.com/mcp"

type mcpOAuthFixture struct {
	*apiFixture
	idp *oidcrptest.IDP
}

func newMCPOAuthFixture(t *testing.T) *mcpOAuthFixture {
	t.Helper()
	idp := oidcrptest.New(t)
	return &mcpOAuthFixture{apiFixture: startAPIFixture(t, idp), idp: idp}
}

func (f *mcpOAuthFixture) linkedPerson(t *testing.T, subject string, roles map[*org.Org]member.Role) *user.User {
	t.Helper()
	ctx := t.Context()
	u, err := f.svcs.Users.Create(ctx, user.CreateRequest{Email: subject + "@example.com", Name: subject, Password: "quiet-forest-lamp-9"})
	require.NoError(t, err)
	linked, err := f.svcs.Users.LinkIDP(ctx, u.ID, f.idp.Issuer(), subject)
	require.NoError(t, err)
	require.True(t, linked)
	for o, role := range roles {
		_, err = f.svcs.Members.Add(ctx, o.ID, u.ID, role)
		require.NoError(t, err)
	}
	return u
}

func (f *mcpOAuthFixture) token(subject, audience string, scopes ...string) string {
	return f.idp.AccessToken(map[string]any{"sub": subject, "aud": audience, "scope": strings.Join(scopes, " ")})
}

func toolCall(name string, args map[string]any) string {
	raw, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": name, "arguments": args},
	})
	if err != nil {
		panic(err)
	}
	return string(raw)
}

func TestMCPOAuth_TokenCallsATool(t *testing.T) {
	f := newMCPOAuthFixture(t)
	acme := f.newOrg(t, "acme-org")
	f.linkedPerson(t, "sub-ada", map[*org.Org]member.Role{acme: member.RoleAdmin})
	tok := f.token("sub-ada", mcpOAuthAudience, "member:read")

	status, _, body := f.mcpCall(t, tok, toolCall("member_list", map[string]any{"orgId": acme.PublicID}))
	require.Equal(t, http.StatusOK, status, string(body))
	assert.NotContains(t, string(body), `"isError":true`)
	assert.Contains(t, string(body), "sub-ada@example.com")

	status, _, body = f.mcpCall(t, tok, toolCall("auth_whoami", map[string]any{}))
	require.Equal(t, http.StatusOK, status, string(body))
	assert.Contains(t, string(body), `\"source\":\"token\"`)
	assert.Contains(t, string(body), acme.PublicID)
}

// SECURITY: RFC 8707: a token minted for another resource is refused.
func TestMCPOAuth_TokenForAnotherResourceIs401(t *testing.T) {
	f := newMCPOAuthFixture(t)
	acme := f.newOrg(t, "acme-org")
	f.linkedPerson(t, "sub-ada", map[*org.Org]member.Role{acme: member.RoleOwner})

	for _, aud := range []string{"https://oagent.example.com/api", "https://other.example.com/mcp"} {
		status, _, body := f.mcpCall(t, f.token("sub-ada", aud, "member:read"), toolCall("auth_whoami", map[string]any{}))
		assert.Equal(t, http.StatusUnauthorized, status, "%s: %s", aud, body)
		assert.Contains(t, string(body), `"code":"GEN002"`)
	}
}

// SECURITY: a person is found only by an existing provider link; no account is made or linked from a token.
func TestMCPOAuth_UnknownSubjectIs401(t *testing.T) {
	f := newMCPOAuthFixture(t)
	status, _, body := f.mcpCall(t, f.token("sub-stranger", mcpOAuthAudience, "member:read"), toolCall("auth_whoami", map[string]any{}))
	assert.Equal(t, http.StatusUnauthorized, status, string(body))
	_, err := f.svcs.Users.ByIDP(t.Context(), f.idp.Issuer(), "sub-stranger")
	assert.True(t, user.IsNotFoundError(err), "no account was linked: %v", err)
}

// SECURITY: someone in two orgs is left untenanted; the token never acts in a guessed org.
func TestMCPOAuth_PersonInTwoOrgsIsUntenanted(t *testing.T) {
	f := newMCPOAuthFixture(t)
	acme, beta := f.newOrg(t, "acme-org"), f.newOrg(t, "beta-org")
	f.linkedPerson(t, "sub-ada", map[*org.Org]member.Role{acme: member.RoleOwner, beta: member.RoleOwner})
	tok := f.token("sub-ada", mcpOAuthAudience, "member:read", "org:read")

	status, _, body := f.mcpCall(t, tok, toolCall("auth_whoami", map[string]any{}))
	require.Equal(t, http.StatusOK, status, string(body))
	assert.NotContains(t, string(body), acme.PublicID)
	assert.NotContains(t, string(body), beta.PublicID)

	for _, o := range []*org.Org{acme, beta} {
		status, _, body := f.mcpCall(t, tok, toolCall("member_list", map[string]any{"orgId": o.PublicID}))
		require.Equal(t, http.StatusOK, status, string(body))
		assert.Contains(t, string(body), `"isError":true`, "%s: %s", o.Slug, body)
		assert.NotContains(t, string(body), "sub-ada@example.com")
	}
}

// SECURITY: granted ∩ role: a member's token granted member:write can't invite.
func TestMCPOAuth_MemberRoleCannotInvite(t *testing.T) {
	f := newMCPOAuthFixture(t)
	acme := f.newOrg(t, "acme-org")
	f.linkedPerson(t, "sub-mia", map[*org.Org]member.Role{acme: member.RoleMember})
	tok := f.token("sub-mia", mcpOAuthAudience, "member:read", "member:write")

	status, _, body := f.mcpCall(t, tok, toolCall("member_invite", map[string]any{"orgId": acme.PublicID, "email": "new@example.com"}))
	require.Equal(t, http.StatusOK, status, string(body))
	assert.Contains(t, string(body), `"isError":true`)
	assert.Contains(t, string(body), `\"scope\":\"member:write\"`)

	var drafts int
	require.NoError(t, f.db.QueryRowContext(t.Context(), `SELECT count(*) FROM invites WHERE org_id = $1`, acme.ID).Scan(&drafts))
	assert.Zero(t, drafts)
}

// SECURITY: a person's token drafts an invite as that person; the draft waits for their confirmation.
func TestMCPOAuth_AdminDraftsAnInviteAsThemself(t *testing.T) {
	f := newMCPOAuthFixture(t)
	acme := f.newOrg(t, "acme-org")
	ada := f.linkedPerson(t, "sub-ada", map[*org.Org]member.Role{acme: member.RoleAdmin})
	tok := f.token("sub-ada", mcpOAuthAudience, "member:write")

	status, _, body := f.mcpCall(t, tok, toolCall("member_invite", map[string]any{"orgId": acme.PublicID, "email": "new@example.com"}))
	require.Equal(t, http.StatusOK, status, string(body))
	assert.NotContains(t, string(body), `"isError":true`)

	var invitedBy uuid.UUID
	require.NoError(t, f.db.QueryRowContext(t.Context(), `SELECT invited_by FROM invites WHERE org_id = $1`, acme.ID).Scan(&invitedBy))
	assert.Equal(t, ada.ID, invitedBy)
}

// SECURITY: a token drafts an invite but never sends it; a person signed in on the web does.
func TestMCPOAuth_TokenCannotSendADraftedInvite(t *testing.T) {
	f := newMCPOAuthFixture(t)
	acme := f.newOrg(t, "acme-org")
	f.linkedPerson(t, "sub-ada", map[*org.Org]member.Role{acme: member.RoleAdmin})
	tok := f.token("sub-ada", mcpOAuthAudience, "member:write")

	status, _, body := f.mcpCall(t, tok, toolCall("member_invite", map[string]any{"orgId": acme.PublicID, "email": "new@example.com"}))
	require.Equal(t, http.StatusOK, status, string(body))
	require.NotContains(t, string(body), `"isError":true`)
	var draftID string
	require.NoError(t, f.db.QueryRowContext(t.Context(), `SELECT public_id FROM invites WHERE org_id = $1`, acme.ID).Scan(&draftID))
	mailBefore := f.queuedMail(t)

	status, _, body = f.mcpCall(t, tok, toolCall("member_invite_confirm", map[string]any{"orgId": acme.PublicID, "inviteId": draftID}))
	require.Equal(t, http.StatusOK, status, string(body))
	assert.Contains(t, string(body), `"isError":true`)
	assert.Contains(t, string(body), `\"code\":\"GEN007\"`)
	assert.Contains(t, string(body), `\"permission\":\"member:write\"`)
	assert.True(t, f.inviteIsDraft(t, draftID), "the draft is still there")
	assert.Equal(t, mailBefore, f.queuedMail(t), "no invite email")

	cookie, _ := f.signInCookie(t, "sub-ada@example.com")
	client := invitev1connect.NewInviteServiceClient(http.DefaultClient, f.url)
	_, err := client.ConfirmDraftInvite(t.Context(), withCookie(connect.NewRequest(&invitev1.ConfirmDraftInviteRequest{OrgId: acme.PublicID, InviteId: draftID}), cookie))
	require.NoError(t, err)
	assert.False(t, f.inviteIsDraft(t, draftID))
	assert.Equal(t, mailBefore+1, f.queuedMail(t))
}

func (f *mcpOAuthFixture) inviteIsDraft(t *testing.T, publicID string) bool {
	t.Helper()
	var draft bool
	require.NoError(t, f.db.QueryRowContext(t.Context(), `SELECT draft FROM invites WHERE public_id = $1`, publicID).Scan(&draft))
	return draft
}

func (f *mcpOAuthFixture) queuedMail(t *testing.T) int {
	t.Helper()
	var n int
	require.NoError(t, f.db.QueryRowContext(t.Context(), `SELECT count(*) FROM mail_queue`).Scan(&n))
	return n
}

// SECURITY: /api takes no provider tokens.
func TestMCPOAuth_TokenIsRefusedOnAPI(t *testing.T) {
	f := newMCPOAuthFixture(t)
	acme := f.newOrg(t, "acme-org")
	f.linkedPerson(t, "sub-ada", map[*org.Org]member.Role{acme: member.RoleOwner})

	client := authv1connect.NewAuthServiceClient(http.DefaultClient, f.url)
	_, err := client.Whoami(t.Context(), withBearer(connect.NewRequest(&authv1.WhoamiRequest{}), f.token("sub-ada", mcpOAuthAudience, "apikey:write")))
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err), "%v", err)
}

func TestMCPOAuth_KeyStillWorks(t *testing.T) {
	f := newMCPOAuthFixture(t)
	key := f.mint(t, apikey.KindOrg, "agent:read")

	status, _, body := f.mcpCall(t, key, toolCall("auth_whoami", map[string]any{}))
	require.Equal(t, http.StatusOK, status, string(body))
	assert.Contains(t, string(body), `\"source\":\"apikey\"`)
}

func TestMCPOAuth_BasicStillWorks(t *testing.T) {
	idp := oidcrptest.New(t)
	cfg := devLocalConfig()
	cfg.Auth = config.AuthConfig{Mode: config.AuthModeBasic, Basic: config.BasicConfig{User: "ops", Password: "basic-password"}}
	cfg.HTTP.BaseURL = "https://oagent.example.com"
	cfg.OIDC = config.OIDCConfig{Issuer: idp.Issuer(), ClientID: "oagent", Scopes: []string{"openid", "email"}, ButtonLabel: "Continue with SSO"}
	cfg.MCP.Enabled = true
	l := newLockdown(t, cfg, idp)
	require.NotNil(t, l.svcs.MCPTokens, "MCP OAuth is on")

	h := mcpJSON()
	h.Set("Authorization", basicAuth("ops", "basic-password"))
	rec := l.do(http.MethodPost, "/mcp", h, toolCall("auth_whoami", map[string]any{}))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), `\"source\":\"basic\"`)

	h.Set("Authorization", basicAuth("ops", "wrong-password"))
	rec = l.do(http.MethodPost, "/mcp", h, toolCall("auth_whoami", map[string]any{}))
	assert.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())
}

func TestMCPOAuth_OffWithoutIssuer(t *testing.T) {
	f := newAPIFixture(t)
	idp := oidcrptest.New(t)
	tok := idp.AccessToken(map[string]any{"aud": "http://localhost:5150/mcp", "scope": "member:read"})

	status, _, body := f.mcpCall(t, tok, toolCall("auth_whoami", map[string]any{}))
	assert.Equal(t, http.StatusUnauthorized, status, string(body))
	assert.Nil(t, f.svcs.MCPTokens)
}

const mcpOAuthMetadata = "https://oagent.example.com/.well-known/oauth-protected-resource/mcp"

func TestMCPOAuth_ServesProtectedResourceMetadata(t *testing.T) {
	f := newMCPOAuthFixture(t)
	resp, err := http.Get(strings.TrimSuffix(f.url, "/api") + "/.well-known/oauth-protected-resource/mcp")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var doc struct {
		Resource             string   `json:"resource"`
		AuthorizationServers []string `json:"authorization_servers"`
		ScopesSupported      []string `json:"scopes_supported"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&doc))
	assert.Equal(t, mcpOAuthAudience, doc.Resource)
	assert.Equal(t, []string{f.idp.Issuer()}, doc.AuthorizationServers)
	assert.Contains(t, doc.ScopesSupported, "agent:read")
	assert.Contains(t, doc.ScopesSupported, "member:write")
	assert.NotContains(t, doc.ScopesSupported, "apikey:write", "no token may hold it")
	assert.NotContains(t, doc.ScopesSupported, "@authenticated")
}

// SECURITY: RFC 6750 Section 3.1: the challenge names the metadata; only the error code tells "no token" from "refused".
func TestMCPOAuth_ChallengeNamesTheMetadata(t *testing.T) {
	f := newMCPOAuthFixture(t)

	status, header, _ := f.mcpCall(t, "", toolCall("auth_whoami", map[string]any{}))
	assert.Equal(t, http.StatusUnauthorized, status)
	assert.Equal(t, `Bearer resource_metadata="`+mcpOAuthMetadata+`"`, header.Get("WWW-Authenticate"))

	for _, bearer := range []string{"oagent_nope", f.token("sub-stranger", mcpOAuthAudience, "member:read")} {
		status, header, _ = f.mcpCall(t, bearer, toolCall("auth_whoami", map[string]any{}))
		assert.Equal(t, http.StatusUnauthorized, status)
		assert.Equal(t, `Bearer resource_metadata="`+mcpOAuthMetadata+`", error="invalid_token"`, header.Get("WWW-Authenticate"))
	}
}

func TestMCPOAuth_TokenWithoutTheToolScopeGetsAStepUp(t *testing.T) {
	f := newMCPOAuthFixture(t)
	acme := f.newOrg(t, "acme-org")
	f.linkedPerson(t, "sub-ada", map[*org.Org]member.Role{acme: member.RoleAdmin})

	status, header, body := f.mcpCall(t, f.token("sub-ada", mcpOAuthAudience, "member:read"),
		toolCall("member_invite", map[string]any{"orgId": acme.PublicID, "email": "new@example.com"}))
	assert.Equal(t, http.StatusForbidden, status, string(body))
	assert.Equal(t, `Bearer resource_metadata="`+mcpOAuthMetadata+`", error="insufficient_scope", scope="member:write"`, header.Get("WWW-Authenticate"))
}

// NOTE: no challenge on an outage, so the client keeps its token.
func TestMCPOAuth_ProviderDownIs503(t *testing.T) {
	f := newMCPOAuthFixture(t)
	acme := f.newOrg(t, "acme-org")
	f.linkedPerson(t, "sub-ada", map[*org.Org]member.Role{acme: member.RoleAdmin})
	tok := f.token("sub-ada", mcpOAuthAudience, "member:read")
	f.idp.Close()

	status, header, body := f.mcpCall(t, tok, toolCall("auth_whoami", map[string]any{}))
	assert.Equal(t, http.StatusServiceUnavailable, status, string(body))
	assert.Empty(t, header.Get("WWW-Authenticate"))
	assert.Contains(t, string(body), `"code":"GEN009"`)
}
