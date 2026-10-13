package boot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/config"
	agentv1mcp "altalune.id/oagent/gen/go/agent/v1/agentv1mcp"
	authv1mcp "altalune.id/oagent/gen/go/auth/v1/authv1mcp"
	orgv1connect "altalune.id/oagent/gen/go/org/v1/orgv1connect"
	orgv1mcp "altalune.id/oagent/gen/go/org/v1/orgv1mcp"
	projectv1mcp "altalune.id/oagent/gen/go/project/v1/projectv1mcp"
	webhookv1mcp "altalune.id/oagent/gen/go/webhook/v1/webhookv1mcp"
	"altalune.id/oagent/internal/apikey"
	"altalune.id/oagent/internal/auth"
	"altalune.id/oagent/internal/kernel/authz"
	"altalune.id/oagent/internal/kernel/capabilities"
	"altalune.id/oagent/internal/kernel/session"
	mcpinternal "altalune.id/oagent/internal/mcp"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/permission"
	"altalune.id/oagent/internal/testutil/fakes"
	rootmcp "altalune.id/oagent/mcp"
	"altalune.id/oagent/scope"
)

func TestBuildMCPServer_RegistersWhoami(t *testing.T) {
	srv := buildMCPServer(config.MCPConfig{Enabled: true}, &Services{Orgs: org.NewService(&fakes.OrgStore{}, nil)}, nil)
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

func TestBuildMCPHandler(t *testing.T) {
	svcs := &Services{Orgs: org.NewService(&fakes.OrgStore{}, nil), KeyScheme: apikey.NewScheme(""), KeyAuthn: &apikey.Authenticator{}}
	assert.Nil(t, buildMCPHandler(nil, svcs, nil))

	h := buildMCPHandler(buildMCPServer(config.MCPConfig{Enabled: true}, svcs, nil), svcs, nil)
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
	srv := buildMCPServer(config.MCPConfig{Enabled: true}, &Services{Orgs: org.NewService(&fakes.OrgStore{}, nil)}, nil)
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
	orgv1mcp.RegisterOrgServiceTools(reg, org.NewControlPlane(org.NewService(&fakes.OrgStore{}, nil)), func(procedure string) []string {
		if procedure == orgv1connect.OrgServiceGetOrgProcedure {
			return nil
		}
		return permission.For(procedure)
	})
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
