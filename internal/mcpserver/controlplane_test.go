package mcpserver_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	"altalune.id/oagent/apperror"
	mcpserverv1 "altalune.id/oagent/gen/go/mcpserver/v1"
	mcpserverv1connect "altalune.id/oagent/gen/go/mcpserver/v1/mcpserverv1connect"
	"altalune.id/oagent/gen/go/mcpserver/v1/mcpserverv1mcp"
	"altalune.id/oagent/internal/mcpserver"
	"altalune.id/oagent/internal/testutil/fakes"
)

type cpFixture struct {
	store      *fakes.MCPServerStore
	refs       *fakes.PublicIDs
	svc        *mcpserver.Service
	cp         *mcpserver.McpServerService
	client     mcpserverv1connect.McpServerServiceClient
	orgID      uuid.UUID
	projectID  uuid.UUID
	orgPub     string
	projectPub string
}

func newCPFixture(t *testing.T) *cpFixture {
	t.Helper()
	f := &cpFixture{store: &fakes.MCPServerStore{}, refs: fakes.NewPublicIDs(), orgID: uuid.New(), projectID: uuid.New()}
	f.orgPub = f.refs.Org(f.orgID)
	f.projectPub = f.refs.Project(f.projectID, f.orgID)
	f.svc = mcpserver.NewService(f.store, fakes.Transactor{}, testUnexpected()).WithSealer(testSealer(t))
	f.cp = mcpserver.NewControlPlane(f.svc, f.refs)
	_, handler := mcpserverv1connect.NewMcpServerServiceHandler(f.cp, connect.WithInterceptors(fakes.OperatorInterceptor()))
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	f.client = mcpserverv1connect.NewMcpServerServiceClient(http.DefaultClient, ts.URL)
	return f
}

func createReq(orgPub, projectPub string) *mcpserverv1.CreateMcpServerRequest {
	return &mcpserverv1.CreateMcpServerRequest{
		OrgId: orgPub, ProjectId: projectPub, Name: "yasaku", Url: "https://yasaku.example.com/mcp",
		AuthKind: mcpserverv1.McpServerAuthKind_MCP_SERVER_AUTH_KIND_HEADER, AuthHeader: "X-Api-Key", AuthValue: authValue,
		TimeoutMs: 5000, Trusted: true,
	}
}

func TestMcpServerService_CreateReturnsNoSecret(t *testing.T) {
	f := newCPFixture(t)
	resp, err := f.client.CreateMcpServer(context.Background(), connect.NewRequest(createReq(f.orgPub, f.projectPub)))
	require.NoError(t, err)
	s := resp.Msg.GetMcpServer()
	assert.True(t, s.GetAuthSet())
	assert.Equal(t, f.orgPub, s.GetOrgId())
	assert.Equal(t, f.projectPub, s.GetProjectId())
	assert.Equal(t, "yasaku", s.GetName())
	assert.Equal(t, "https://yasaku.example.com/mcp", s.GetUrl())
	assert.Equal(t, mcpserverv1.McpServerAuthKind_MCP_SERVER_AUTH_KIND_HEADER, s.GetAuthKind())
	assert.Equal(t, "X-Api-Key", s.GetAuthHeader())
	assert.Equal(t, int32(5000), s.GetTimeoutMs())
	assert.True(t, s.GetTrusted())
	raw, err := protojson.Marshal(resp.Msg)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), authValue)
	require.Len(t, f.store.Servers, 1)
	for id, saved := range f.store.Servers {
		assert.Equal(t, s.GetId(), saved.PublicID)
		assert.NotEqual(t, id.String(), s.GetId())
	}
}

func TestMcpServerService_GetUpdateListDelete(t *testing.T) {
	f := newCPFixture(t)
	ctx := context.Background()
	created, err := f.client.CreateMcpServer(ctx, connect.NewRequest(createReq(f.orgPub, f.projectPub)))
	require.NoError(t, err)
	id := created.Msg.GetMcpServer().GetId()

	got, err := f.client.GetMcpServer(ctx, connect.NewRequest(&mcpserverv1.GetMcpServerRequest{Id: id}))
	require.NoError(t, err)
	assert.Equal(t, f.orgPub, got.Msg.GetMcpServer().GetOrgId())

	upd := &mcpserverv1.UpdateMcpServerRequest{
		Id: id, Url: "https://yasaku.example.com/mcp", AuthKind: mcpserverv1.McpServerAuthKind_MCP_SERVER_AUTH_KIND_HEADER,
		AuthHeader: "X-Other", TimeoutMs: 9000,
	}
	updated, err := f.client.UpdateMcpServer(ctx, connect.NewRequest(upd))
	require.NoError(t, err)
	assert.Equal(t, "X-Other", updated.Msg.GetMcpServer().GetAuthHeader())
	assert.Equal(t, "yasaku", updated.Msg.GetMcpServer().GetName(), "the name never changes")
	assert.True(t, updated.Msg.GetMcpServer().GetAuthSet())

	upd.Url = "https://other.example.com/mcp"
	_, err = f.cp.UpdateMcpServer(fakes.OperatorContext(ctx), connect.NewRequest(upd))
	assert.Equal(t, apperror.CodeMcpServerSecretRequired, codeOf(t, err), "a new URL needs the credential again")

	upd.AuthKind = mcpserverv1.McpServerAuthKind_MCP_SERVER_AUTH_KIND_NONE
	updated, err = f.client.UpdateMcpServer(ctx, connect.NewRequest(upd))
	require.NoError(t, err)
	assert.False(t, updated.Msg.GetMcpServer().GetAuthSet())
	assert.Empty(t, updated.Msg.GetMcpServer().GetAuthHeader())

	list, err := f.client.ListMcpServers(ctx, connect.NewRequest(&mcpserverv1.ListMcpServersRequest{ProjectId: f.projectPub}))
	require.NoError(t, err)
	require.Len(t, list.Msg.GetMcpServers(), 1)
	assert.Equal(t, f.orgPub, list.Msg.GetMcpServers()[0].GetOrgId())

	_, err = f.client.DeleteMcpServer(ctx, connect.NewRequest(&mcpserverv1.DeleteMcpServerRequest{Id: id}))
	require.NoError(t, err)
	assert.Empty(t, f.store.Servers)
}

func TestMcpServerService_ListToolsSuggestsApproval(t *testing.T) {
	f := newCPFixture(t)
	ts := httptest.NewServer(mcpHandler(readOnlyTool("expense_list"), &mcp.Tool{Name: "expense_create", Description: "Record", InputSchema: objectSchema()}))
	t.Cleanup(ts.Close)
	f.svc.WithAllowInsecureURL().WithHTTPClient(mcpserver.NewProbeClient(true))
	req := createReq(f.orgPub, f.projectPub)
	req.Url = ts.URL + "/mcp"
	req.AuthKind = mcpserverv1.McpServerAuthKind_MCP_SERVER_AUTH_KIND_NONE
	created, err := f.client.CreateMcpServer(context.Background(), connect.NewRequest(req))
	require.NoError(t, err)

	resp, err := f.client.ListMcpServerTools(context.Background(), connect.NewRequest(&mcpserverv1.ListMcpServerToolsRequest{Id: created.Msg.GetMcpServer().GetId()}))
	require.NoError(t, err)
	got := map[string]*mcpserverv1.McpServerTool{}
	for _, tool := range resp.Msg.GetTools() {
		got[tool.GetName()] = tool
	}
	require.Len(t, got, 2)
	assert.True(t, got["expense_list"].GetReadOnly())
	assert.False(t, got["expense_list"].GetRequireApproval())
	assert.True(t, got["expense_create"].GetRequireApproval())
	assert.True(t, got["expense_create"].GetGrantable())
	assert.Equal(t, "Record", got["expense_create"].GetDescription())
	assert.Equal(t, "object", got["expense_create"].GetInputSchema().GetFields()["type"].GetStringValue())
}

func TestMcpServerService_GetWithAnotherIDIsNotFound(t *testing.T) {
	f := newCPFixture(t)
	_, err := f.client.CreateMcpServer(context.Background(), connect.NewRequest(createReq(f.orgPub, f.projectPub)))
	require.NoError(t, err)
	var rowID uuid.UUID
	for id := range f.store.Servers {
		rowID = id
	}
	for _, id := range []string{f.projectPub, rowID.String()} {
		_, err := f.cp.GetMcpServer(fakes.OperatorContext(context.Background()), connect.NewRequest(&mcpserverv1.GetMcpServerRequest{Id: id}))
		assert.Equal(t, apperror.CodeMcpServerNotFound, codeOf(t, err), "id %s", id)
	}
}

func TestMcpServerService_CreateWithAProjectOfAnotherOrgIsNotFound(t *testing.T) {
	f := newCPFixture(t)
	otherOrg := f.refs.Org(uuid.New())
	for _, org := range []string{otherOrg, "unknown-org-id-000000", f.orgID.String()} {
		_, err := f.cp.CreateMcpServer(fakes.OperatorContext(context.Background()), connect.NewRequest(createReq(org, f.projectPub)))
		assert.Equal(t, apperror.CodeMcpServerNotFound, codeOf(t, err), "org %s", org)
	}
	assert.Empty(t, f.store.Servers)
}

// SECURITY: a server credential never travels through an MCP tool, so the tools that take one are not generated.
func TestMcpServerMCPTools_NeverTakeASecret(t *testing.T) {
	names := mcpserverv1mcp.McpServerServiceToolNames()
	assert.ElementsMatch(t, []string{"mcp_server_get", "mcp_server_list", "mcp_server_delete", "mcp_server_tools"}, names)
	assert.NotContains(t, names, "mcp_server_create")
	assert.NotContains(t, names, "mcp_server_update")
}
