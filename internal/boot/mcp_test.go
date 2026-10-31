package boot

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/altalune-id/oidcrp/oidcrptest"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/config"
	agentv1mcp "altalune.id/oagent/gen/go/agent/v1/agentv1mcp"
	authv1mcp "altalune.id/oagent/gen/go/auth/v1/authv1mcp"
	invitev1mcp "altalune.id/oagent/gen/go/invite/v1/invitev1mcp"
	memberv1mcp "altalune.id/oagent/gen/go/member/v1/memberv1mcp"
	orgv1connect "altalune.id/oagent/gen/go/org/v1/orgv1connect"
	orgv1mcp "altalune.id/oagent/gen/go/org/v1/orgv1mcp"
	projectv1mcp "altalune.id/oagent/gen/go/project/v1/projectv1mcp"
	webhookv1mcp "altalune.id/oagent/gen/go/webhook/v1/webhookv1mcp"
	"altalune.id/oagent/internal/apikey"
	"altalune.id/oagent/internal/auth"
	"altalune.id/oagent/internal/controlplane"
	"altalune.id/oagent/internal/kernel/authz"
	"altalune.id/oagent/internal/kernel/capabilities"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/kernel/tokens"
	mcpinternal "altalune.id/oagent/internal/mcp"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/permission"
	"altalune.id/oagent/internal/testutil/fakes"
	rootmcp "altalune.id/oagent/mcp"
	"altalune.id/oagent/scope"
)

func TestBuildMCPServer_RegistersWhoami(t *testing.T) {
	srv := buildMCPServer(config.MCPConfig{Enabled: true}, &Services{Orgs: org.NewService(&fakes.OrgStore{}, fakes.Transactor{}, nil)}, nil)
	require.NotNil(t, srv)
	assert.Contains(t, srv.Registry().Names(), authv1mcp.AuthWhoamiToolName)
}

// SECURITY: the MCP chain never includes the session cookie authenticator.
func TestMCPChain(t *testing.T) {
	local := &Services{Caps: capabilities.Capabilities{LocalLogin: true}, KeyAuthn: &apikey.Authenticator{}, Auth: &auth.Service{}}
	chain := mcpChain(local)
	require.Len(t, chain, 1)
	assert.IsType(t, &apikey.Authenticator{}, chain[0])

	basic := &Services{Caps: capabilities.Capabilities{BasicAuth: true}, KeyAuthn: &apikey.Authenticator{}, Auth: &auth.Service{}}
	chain = mcpChain(basic)
	require.Len(t, chain, 2)
	assert.IsType(t, auth.BasicAuthenticator{}, chain[1])
}

func TestMCPChain_TokensWhenMCPOAuthIsOn(t *testing.T) {
	svcs := &Services{KeyAuthn: &apikey.Authenticator{}, Auth: &auth.Service{}, MCPTokens: fakeTokens{}}
	chain := mcpChain(svcs)
	require.Len(t, chain, 2)
	assert.Equal(t, auth.TokenAuthenticator{Verifier: fakeTokens{}, Auth: svcs.Auth}, chain[1])
}

type fakeTokens struct{}

func (fakeTokens) Verify(context.Context, string) (session.Principal, error) {
	return session.Principal{}, &tokens.InvalidTokenError{}
}

func mcpOAuthConfig(issuer string) config.Config {
	cfg := devLocalConfig()
	cfg.HTTP.BaseURL = "https://oagent.example.com"
	cfg.MCP.Enabled = true
	cfg.OIDC = config.OIDCConfig{Issuer: issuer, ClientID: "oagent"}
	return cfg
}

func TestMCPAudience(t *testing.T) {
	cfg := mcpOAuthConfig("https://idp.example.com")
	assert.Equal(t, "https://oagent.example.com/mcp", mcpAudience(cfg))
	cfg.HTTP.BasePath = "/oagent"
	assert.Equal(t, "https://oagent.example.com/oagent/mcp", mcpAudience(cfg))
	cfg.HTTP.BaseURL = ""
	assert.Equal(t, "http://localhost:5150/oagent/mcp", mcpAudience(cfg), "development falls back to the listen address")
	cfg.MCP.Audience = "https://proxy.example.com/mcp"
	assert.Equal(t, "https://proxy.example.com/mcp", mcpAudience(cfg))
}

func TestBuildMCPTokens_OnlyWithMCPOAuth(t *testing.T) {
	for name, cfg := range map[string]config.Config{
		"no issuer": devLocalConfig(),
		"mcp off":   func() config.Config { c := mcpOAuthConfig("https://idp.example.com"); c.MCP.Enabled = false; return c }(),
	} {
		v, err := buildMCPTokens(cfg, nil)
		require.NoError(t, err, name)
		assert.Nil(t, v, name)
	}
}

func TestBuildMCPTokens_TakesTheClockSkew(t *testing.T) {
	idp := oidcrptest.New(t)
	now := time.Now()
	late := idp.AccessToken(map[string]any{
		"aud": "https://oagent.example.com/mcp", "iat": now.Add(-time.Hour).Unix(), "exp": now.Add(-45 * time.Second).Unix(),
	})
	for skew, ok := range map[time.Duration]bool{time.Minute: true, 10 * time.Second: false} {
		cfg := mcpOAuthConfig(idp.Issuer())
		cfg.MCP.ClockSkew = skew
		v, err := buildMCPTokens(cfg, idp.Client())
		require.NoError(t, err)
		_, err = v.Verify(t.Context(), late)
		assert.Equal(t, ok, err == nil, "skew %s: %v", skew, err)
	}
}

// SECURITY: RFC 8707: the verifier takes only tokens minted for this MCP endpoint.
func TestBuildMCPTokens_PinsTheAudience(t *testing.T) {
	idp := oidcrptest.New(t)
	v, err := buildMCPTokens(mcpOAuthConfig(idp.Issuer()), idp.Client())
	require.NoError(t, err)
	require.NotNil(t, v)

	p, err := v.Verify(t.Context(), idp.AccessToken(map[string]any{"aud": "https://oagent.example.com/mcp"}))
	require.NoError(t, err)
	assert.Equal(t, session.SourceToken, p.Source)

	_, err = v.Verify(t.Context(), idp.AccessToken(map[string]any{"aud": "https://oagent.example.com/api"}))
	assert.ErrorAs(t, err, new(*tokens.InvalidTokenError))
}

// SECURITY: the RFC 9728 document offers only catalog scopes a token may hold, whatever a tool declares.
func TestRegisteredScopes_OfferOnlyTokenScopes(t *testing.T) {
	noop := func(context.Context, json.RawMessage) (json.RawMessage, error) { return nil, nil }
	reg := rootmcp.NewRegistry()
	reg.Register(rootmcp.ToolSpec{Name: "agent_list", Scopes: []string{scope.AgentRead}, Handler: noop}, nil)
	reg.Register(rootmcp.ToolSpec{Name: "agent_get", Scopes: []string{scope.AgentRead}, Handler: noop}, nil)
	reg.Register(rootmcp.ToolSpec{Name: "member_invite", Scopes: []string{scope.MemberWrite}, Handler: noop}, nil)
	reg.Register(rootmcp.ToolSpec{Name: "apikey_create", Scopes: []string{scope.ApikeyWrite}, Handler: noop}, nil)
	reg.Register(rootmcp.ToolSpec{Name: "auth_whoami", Scopes: []string{authz.AnyAuthenticated}, Handler: noop}, nil)

	assert.Equal(t, []string{scope.AgentRead, scope.MemberWrite}, registeredScopes(reg))
	assert.Equal(t, []string{scope.MemberWrite}, toolScopes(reg)("member_invite"))
	assert.Nil(t, toolScopes(reg)("nope"))
}

func TestBuildMCPResource(t *testing.T) {
	srv := buildMCPServer(config.MCPConfig{Enabled: true}, &Services{Orgs: org.NewService(&fakes.OrgStore{}, fakes.Transactor{}, nil)}, nil)

	log, buf := bufferLogger()
	r, docs, err := buildMCPResource(devLocalConfig(), srv, log)
	require.NoError(t, err)
	assert.Equal(t, tokens.Resource{}, r, "off without MCP OAuth")
	assert.Nil(t, docs)
	assert.Empty(t, buf.String())

	cfg := mcpOAuthConfig("https://idp.example.com")
	cfg.HTTP.BasePath = "/oagent"
	r, docs, err = buildMCPResource(cfg, srv, log)
	require.NoError(t, err)
	assert.Equal(t, "https://oagent.example.com/oagent/mcp", r.Identifier)
	assert.Contains(t, buf.String(), `msg="boot: mcp oauth on" audience=https://oagent.example.com/oagent/mcp`)
	assert.Contains(t, buf.String(), "metadata=/.well-known/oauth-protected-resource/oagent/mcp")
	assert.Contains(t, buf.String(), "issuer=https://idp.example.com")
	require.Contains(t, docs, "/.well-known/oauth-protected-resource/oagent/mcp")

	rec := httptest.NewRecorder()
	docs[r.MetadataPath].ServeHTTP(rec, httptest.NewRequest(http.MethodGet, r.MetadataPath, nil))
	require.Equal(t, http.StatusOK, rec.Code)
	var doc struct {
		Resource             string   `json:"resource"`
		AuthorizationServers []string `json:"authorization_servers"`
		ScopesSupported      []string `json:"scopes_supported"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &doc))
	assert.Equal(t, r.Identifier, doc.Resource)
	assert.Equal(t, []string{"https://idp.example.com"}, doc.AuthorizationServers)
	assert.Contains(t, doc.ScopesSupported, scope.MemberWrite)
	assert.NotContains(t, doc.ScopesSupported, scope.ApikeyWrite)

	cfg.MCP.Audience = "urn:oagent:mcp"
	_, _, err = buildMCPResource(cfg, srv, log)
	assert.ErrorAs(t, err, new(*tokens.ResourceInvalidError))
}

func TestBuildMCPHandler(t *testing.T) {
	svcs := &Services{Orgs: org.NewService(&fakes.OrgStore{}, fakes.Transactor{}, nil), KeyScheme: apikey.NewScheme(""), KeyAuthn: &apikey.Authenticator{}}
	assert.Nil(t, buildMCPHandler(nil, svcs, tokens.Resource{}, nil))

	h := buildMCPHandler(buildMCPServer(config.MCPConfig{Enabled: true}, svcs, nil), svcs, tokens.Resource{}, nil)
	require.NotNil(t, h)

	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Token nope")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Equal(t, "Bearer", rec.Header().Get("WWW-Authenticate"))
}

func TestApp_RunMCPActsAsSystem(t *testing.T) {
	seen := make(chan session.Principal, 1)
	reg := rootmcp.NewRegistry()
	reg.Register(rootmcp.ToolSpec{Name: "probe", Scopes: []string{scope.AgentRead}, Handler: func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
		seen <- session.PrincipalFrom(ctx)
		return json.RawMessage(`{}`), nil
	}}, nil)
	app := &App{MCP: rootmcp.NewServer(rootmcp.WithRegistry(reg), rootmcp.WithScopes(mcpinternal.ScopesFromContext))}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	clientT, serverT := sdkmcp.NewInMemoryTransports()
	go func() { _ = app.runMCP(ctx, serverT) }()
	cs, err := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "t", Version: "v0"}, nil).Connect(ctx, clientT, nil)
	require.NoError(t, err)
	defer func() { _ = cs.Close() }()

	res, err := cs.CallTool(ctx, &sdkmcp.CallToolParams{Name: "probe"})
	require.NoError(t, err)
	require.False(t, res.IsError, "%+v", res.Content)
	assert.Equal(t, session.System(), <-seen)
}

// SECURITY: every published tool takes its permissions from permission.Table, through its RPC.
func TestMCPTools_PermissionsComeFromTheTable(t *testing.T) {
	srv := buildMCPServer(config.MCPConfig{Enabled: true}, &Services{Orgs: org.NewService(&fakes.OrgStore{}, fakes.Transactor{}, nil)}, nil)
	require.NotNil(t, srv)
	inTable := map[string]bool{}
	for _, perms := range permission.Table() {
		for _, p := range perms {
			inTable[p] = true
		}
	}
	want := map[string][]string{
		orgv1mcp.OrgCreateToolName:              {scope.OrgWrite},
		orgv1mcp.OrgGetToolName:                 {scope.OrgRead},
		orgv1mcp.OrgListToolName:                {scope.OrgRead},
		projectv1mcp.ProjectCreateToolName:      {scope.ProjectWrite},
		projectv1mcp.ProjectGetToolName:         {scope.ProjectRead},
		projectv1mcp.ProjectListToolName:        {scope.ProjectRead},
		agentv1mcp.AgentCreateToolName:          {scope.AgentWrite},
		agentv1mcp.AgentGetToolName:             {scope.AgentRead},
		agentv1mcp.AgentListToolName:            {scope.AgentRead},
		agentv1mcp.AgentUpdateToolName:          {scope.AgentWrite},
		agentv1mcp.AgentDeleteToolName:          {scope.AgentWrite},
		webhookv1mcp.WebhookCreateToolName:      {scope.WebhookWrite},
		webhookv1mcp.WebhookListToolName:        {scope.WebhookRead},
		webhookv1mcp.WebhookDeliveriesToolName:  {scope.WebhookRead},
		webhookv1mcp.WebhookRetryFailedToolName: {scope.WebhookWrite},
		memberv1mcp.MemberListToolName:          {scope.MemberRead},
		invitev1mcp.MemberInviteToolName:        {scope.MemberWrite},
		invitev1mcp.MemberInviteConfirmToolName: {scope.MemberWrite},
		invitev1mcp.MemberInviteDiscardToolName: {scope.MemberWrite},
		authv1mcp.AuthWhoamiToolName:            {authz.AnyAuthenticated},
	}
	names := srv.Registry().Names()
	assert.Len(t, names, len(want), "the published tool list")
	for _, name := range names {
		spec, ok := srv.Registry().Spec(name)
		require.True(t, ok)
		assert.NotEmpty(t, spec.Scopes, "%s has no permission", name)
		for _, p := range spec.Scopes {
			assert.True(t, inTable[p], "%s needs %q, which permission.Table does not name", name, p)
		}
		assert.Equal(t, want[name], spec.Scopes, name)
	}
}

// SECURITY: a generated tool whose RPC is missing from the table is denied and hidden.
func TestMCPTools_UnmappedProcedureIsDenied(t *testing.T) {
	reg := rootmcp.NewRegistry()
	orgv1mcp.RegisterOrgServiceTools(reg, org.NewControlPlane(org.NewService(&fakes.OrgStore{}, fakes.Transactor{}, nil)), func(procedure string) []string {
		if procedure == orgv1connect.OrgServiceGetOrgProcedure {
			return nil
		}
		return permission.For(procedure)
	}, (*controlplane.Validator)(nil).Validate)
	app := &App{MCP: rootmcp.NewServer(rootmcp.WithRegistry(reg), rootmcp.WithScopes(mcpinternal.ScopesFromContext))}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	clientT, serverT := sdkmcp.NewInMemoryTransports()
	go func() { _ = app.runMCP(ctx, serverT) }()
	cs, err := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "t", Version: "v0"}, nil).Connect(ctx, clientT, nil)
	require.NoError(t, err)
	defer func() { _ = cs.Close() }()

	list, err := cs.ListTools(ctx, nil)
	require.NoError(t, err)
	var listed []string
	for _, tool := range list.Tools {
		listed = append(listed, tool.Name)
	}
	assert.Equal(t, []string{orgv1mcp.OrgCreateToolName, orgv1mcp.OrgListToolName}, listed, "system sees every mapped tool, not the unmapped one")

	res, err := cs.CallTool(ctx, &sdkmcp.CallToolParams{Name: orgv1mcp.OrgGetToolName, Arguments: map[string]any{"id": "x"}})
	require.NoError(t, err)
	assert.True(t, res.IsError, "even the system operator cannot call a tool whose RPC has no permission")
}

// SECURITY: the model must never be offered the tool that sends a drafted invite.
func TestMCPTools_InviteConfirmIsForTheAppOnly(t *testing.T) {
	list := func(appsUI bool) map[string]*sdkmcp.Tool {
		srv := buildMCPServer(config.MCPConfig{Enabled: true, AppsUI: appsUI}, &Services{Orgs: org.NewService(&fakes.OrgStore{}, fakes.Transactor{}, nil)}, nil)
		app := &App{MCP: srv}
		ctx, cancel := context.WithCancel(t.Context())
		t.Cleanup(cancel)
		clientT, serverT := sdkmcp.NewInMemoryTransports()
		go func() { _ = app.runMCP(ctx, serverT) }()
		cs, err := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "t", Version: "v0"}, nil).Connect(ctx, clientT, nil)
		require.NoError(t, err)
		t.Cleanup(func() { _ = cs.Close() })
		res, err := cs.ListTools(ctx, nil)
		require.NoError(t, err)
		out := map[string]*sdkmcp.Tool{}
		for _, tool := range res.Tools {
			out[tool.Name] = tool
		}
		return out
	}

	withApp := list(true)
	for _, name := range []string{invitev1mcp.MemberInviteConfirmToolName, invitev1mcp.MemberInviteDiscardToolName} {
		require.Contains(t, withApp, name)
		ui, ok := withApp[name].Meta["ui"].(map[string]any)
		require.True(t, ok, name)
		assert.Equal(t, []any{"app"}, ui["visibility"], "%s is hidden from the model", name)
	}
	draft, ok := withApp[invitev1mcp.MemberInviteToolName].Meta["ui"].(map[string]any)
	require.True(t, ok)
	assert.NotContains(t, draft, "visibility", "the model may draft")

	without := list(false)
	assert.Contains(t, without, invitev1mcp.MemberInviteToolName)
	assert.NotContains(t, without, invitev1mcp.MemberInviteConfirmToolName, "no app, no confirm tool")
	assert.NotContains(t, without, invitev1mcp.MemberInviteDiscardToolName)
}

// /mcp behind a same-host proxy accepts http.baseURL's host on a loopback connection, and only that one.
func TestMCPPublicHost(t *testing.T) {
	assert.Equal(t, []string{"id.example.com"}, mcpPublicHost(config.HTTPConfig{BaseURL: "https://id.example.com"}))
	assert.Equal(t, []string{"id.example.com"}, mcpPublicHost(config.HTTPConfig{BaseURL: "https://id.example.com:8443"}))
	assert.Nil(t, mcpPublicHost(config.HTTPConfig{}))

	srv := buildMCPServer(config.MCPConfig{Enabled: true}, &Services{Orgs: org.NewService(&fakes.OrgStore{}, fakes.Transactor{}, nil)}, nil,
		mcpPublicHost(config.HTTPConfig{BaseURL: "https://id.example.com"})...)
	for host, want := range map[string]int{"id.example.com": http.StatusOK, "evil.example": http.StatusForbidden} {
		req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(
			`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`))
		req.Host = host
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		req = req.WithContext(context.WithValue(req.Context(), http.LocalAddrContextKey, net.Addr(&net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})))
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		assert.Equal(t, want, rec.Code, host)
	}
}
