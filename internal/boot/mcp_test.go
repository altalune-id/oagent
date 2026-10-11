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
	authv1mcp "altalune.id/oagent/gen/go/auth/v1/authv1mcp"
	"altalune.id/oagent/internal/apikey"
	"altalune.id/oagent/internal/auth"
	"altalune.id/oagent/internal/kernel/capabilities"
	"altalune.id/oagent/internal/kernel/session"
	mcpinternal "altalune.id/oagent/internal/mcp"
	"altalune.id/oagent/internal/org"
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
	reg.Register(rootmcp.ToolSpec{Name: "probe", Scope: scope.AgentRead, Handler: func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
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
