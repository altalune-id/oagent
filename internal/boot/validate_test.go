package boot

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"altalune.id/oagent/config"
	apperrorv1 "altalune.id/oagent/gen/go/apperror/v1"
	orgv1 "altalune.id/oagent/gen/go/org/v1"
	"altalune.id/oagent/internal/controlplane"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/testutil/cptest"
	"altalune.id/oagent/internal/testutil/fakes"
	rootmcp "altalune.id/oagent/mcp"
)

type refusal struct {
	Code       string
	Meta       map[string]string
	Violations []rootmcp.Violation
}

type sameRefusalCase struct {
	name      string
	tool      string
	args      map[string]any
	procedure string
	request   proto.Message
}

func sameRefusalCases() []sameRefusalCase {
	long := strings.Repeat("a", 201)
	return []sameRefusalCase{
		{name: "missing org name", tool: "org_create", args: map[string]any{}, procedure: "/org.v1.OrgService/CreateOrg", request: &orgv1.CreateOrgRequest{}},
		{name: "201-character org name", tool: "org_create", args: map[string]any{"name": long}, procedure: "/org.v1.OrgService/CreateOrg", request: &orgv1.CreateOrgRequest{Name: long}},
		{name: "malformed public id", tool: "org_get", args: map[string]any{"id": "x"}, procedure: "/org.v1.OrgService/GetOrg", request: &orgv1.GetOrgRequest{Id: "x"}},
	}
}

func TestInvalidRequest_SameRefusalOverAPIAndMCP(t *testing.T) {
	svcs := &Services{Orgs: org.NewService(&fakes.OrgStore{}, fakes.Transactor{}, nil).WithCreation(true)}
	var err error
	svcs.Validator, err = controlplane.NewValidator()
	require.NoError(t, err)

	opts := cptest.OperatorOptions()
	opts.Validator = svcs.Validator
	ts := httptest.NewServer(controlplane.New(controlplane.Services{Orgs: svcs.Orgs}, nil, opts).Handler(""))
	t.Cleanup(ts.Close)

	cs := mcpSession(t, buildMCPServer(config.MCPConfig{Enabled: true}, svcs, nil))

	for _, tt := range sameRefusalCases() {
		t.Run(tt.name, func(t *testing.T) {
			overAPI := apiRefusal(t, ts.URL+"/api"+tt.procedure, tt.request)
			overMCP := mcpRefusal(t, cs, tt.tool, tt.args)
			assert.Equal(t, "GEN004", overAPI.Code)
			assert.NotEmpty(t, overAPI.Violations)
			assert.Equal(t, overAPI, overMCP)
		})
	}
}

func mcpSession(t *testing.T, srv *rootmcp.Server) *sdkmcp.ClientSession {
	t.Helper()
	require.NotNil(t, srv)
	app := &App{MCP: srv}
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	clientT, serverT := sdkmcp.NewInMemoryTransports()
	go func() { _ = app.runMCP(ctx, serverT) }()
	cs, err := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "t", Version: "v0"}, nil).Connect(ctx, clientT, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func apiRefusal(t *testing.T, url string, req proto.Message) refusal {
	t.Helper()
	raw, err := protojson.Marshal(req)
	require.NoError(t, err)
	httpReq, err := http.NewRequestWithContext(t.Context(), http.MethodPost, url, bytes.NewReader(raw))
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+fakes.OperatorToken)
	resp, err := http.DefaultClient.Do(httpReq)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	var body struct {
		Details []struct {
			Type  string `json:"type"`
			Value string `json:"value"`
		} `json:"details"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Len(t, body.Details, 1)
	bin, err := base64.RawStdEncoding.DecodeString(strings.TrimRight(body.Details[0].Value, "="))
	require.NoError(t, err)
	det := &apperrorv1.ErrorDetail{}
	require.NoError(t, proto.Unmarshal(bin, det))
	out := refusal{Code: det.GetCode(), Meta: det.GetMeta()}
	for _, v := range det.GetViolations() {
		out.Violations = append(out.Violations, rootmcp.Violation{Field: v.GetField(), Reason: v.GetReason(), Param: v.GetParam()})
	}
	return out
}

func mcpRefusal(t *testing.T, cs *sdkmcp.ClientSession, tool string, args map[string]any) refusal {
	t.Helper()
	res, err := cs.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: tool, Arguments: args})
	require.NoError(t, err)
	require.True(t, res.IsError)
	require.Len(t, res.Content, 1)
	text, ok := res.Content[0].(*sdkmcp.TextContent)
	require.True(t, ok)
	var p rootmcp.ErrorPayload
	require.NoError(t, json.Unmarshal([]byte(text.Text), &p))
	return refusal{Code: p.Code, Meta: p.Meta, Violations: p.Violations}
}
