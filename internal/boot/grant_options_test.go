package boot

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/mcpserver"
	"altalune.id/oagent/internal/modelprovider"
	"altalune.id/oagent/internal/testutil/fakes"
	"altalune.id/oagent/sealer"
)

func TestProviderOptions_InsecureOnlyOutsideProduction(t *testing.T) {
	key, err := sealer.GenerateKey()
	require.NoError(t, err)
	sl, err := sealer.New(key)
	require.NoError(t, err)
	in := modelprovider.Input{Name: "local", Kind: modelprovider.KindOpenAICompatible, BaseURL: "http://127.0.0.1:11434/v1", Models: []modelprovider.Model{{ID: "m"}}}
	for env, wantOK := range map[config.Env]bool{config.EnvProduction: false, config.EnvDevelopment: true} {
		svc := configureModelProviders(modelprovider.NewService(&fakes.ModelProviderStore{}, fakes.Transactor{}, nil).WithSealer(sl), config.Config{Env: env})
		_, err := svc.Create(tenant.Into(t.Context(), tenant.Context{OrgID: uuid.New()}), uuid.New(), in, "sk")
		if wantOK {
			assert.NoError(t, err, env)
			continue
		}
		assert.True(t, modelprovider.IsInvalidBaseURLError(err), "%s: %v", env, err)
	}
}

func TestMCPServerOptions_PrivateOnlyOutsideProduction(t *testing.T) {
	srv := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "local", Version: "1"}, nil)
	srv.AddTool(&sdkmcp.Tool{Name: "t", InputSchema: map[string]any{"type": "object"}},
		func(context.Context, *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
			return &sdkmcp.CallToolResult{}, nil
		})
	ts := httptest.NewServer(sdkmcp.NewStreamableHTTPHandler(func(*http.Request) *sdkmcp.Server { return srv },
		&sdkmcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true}))
	t.Cleanup(ts.Close)
	for env, wantOK := range map[config.Env]bool{config.EnvProduction: false, config.EnvDevelopment: true} {
		orgID := uuid.New()
		s, err := mcpserver.New(orgID, uuid.New(), mcpserver.Input{Name: "local", URL: ts.URL + "/mcp", AuthKind: mcpserver.AuthNone, TimeoutMS: 5000},
			time.Now(), mcpserver.AllowInsecureURL())
		require.NoError(t, err)
		store := &fakes.MCPServerStore{Servers: map[uuid.UUID]*mcpserver.Server{s.ID: s}}
		svc := configureMCPServers(mcpserver.NewService(store, fakes.Transactor{}, nil), config.Config{Env: env}, testLogger())
		_, err = svc.ListTools(tenant.Into(t.Context(), tenant.Context{OrgID: orgID}), s.ID)
		if wantOK {
			assert.NoError(t, err, env)
			continue
		}
		assert.True(t, mcpserver.IsUnreachableError(err), "%s: the probe never dials loopback in production: %v", env, err)
	}
}

func TestMCPServerOptions_HTTPOnlyOutsideProduction(t *testing.T) {
	in := mcpserver.Input{Name: "local", URL: "http://127.0.0.1:5190/mcp", AuthKind: mcpserver.AuthNone, TimeoutMS: 5000}
	for env, wantOK := range map[config.Env]bool{config.EnvProduction: false, config.EnvDevelopment: true} {
		svc := configureMCPServers(mcpserver.NewService(&fakes.MCPServerStore{}, fakes.Transactor{}, nil), config.Config{Env: env}, testLogger())
		_, err := svc.Create(tenant.Into(t.Context(), tenant.Context{OrgID: uuid.New()}), uuid.New(), in, "")
		if wantOK {
			assert.NoError(t, err, env)
			continue
		}
		e, ok := errors.AsType[*mcpserver.InvalidURLError](err)
		require.True(t, ok, "%s: %v", env, err)
		assert.Equal(t, mcpserver.URLHTTPSRequired, e.Reason, "production refuses an http:// server URL")
	}
}

func TestChannelHooksURL_EmptyWithoutAPublicBaseURL(t *testing.T) {
	assert.Empty(t, channelHooksURL(config.HTTPConfig{Addr: ":8080"}), "no localhost URL is handed to OpenWA")
	assert.Equal(t, "https://oagent.example.com/base/hooks/openwa", channelHooksURL(config.HTTPConfig{BaseURL: "https://oagent.example.com/", BasePath: "/base"}))
}
