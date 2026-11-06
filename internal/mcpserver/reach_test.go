package mcpserver_test

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mcpserverv1 "altalune.id/oagent/gen/go/mcpserver/v1"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/mcpserver"
	"altalune.id/oagent/internal/testutil/fakes"
)

func mustServer(t *testing.T, orgID, projectID uuid.UUID) *mcpserver.Server {
	t.Helper()
	in := input(mcpserver.AuthNone)
	in.Name = "s-" + uuid.NewString()[:6]
	s, err := mcpserver.New(orgID, projectID, in, time.Now())
	require.NoError(t, err)
	return s
}

func storeWith(ss ...*mcpserver.Server) *fakes.MCPServerStore {
	st := &fakes.MCPServerStore{Servers: map[uuid.UUID]*mcpserver.Server{}}
	for _, s := range ss {
		st.Servers[s.ID] = s
	}
	return st
}

func TestMcpServerReach_IDOnlyRPCs(t *testing.T) {
	refs := fakes.NewPublicIDs()
	orgA, orgB, prjB := uuid.New(), uuid.New(), uuid.New()
	refs.Org(orgA)
	orgBPub := refs.Org(orgB)
	prjBPub := refs.Project(prjB, orgB)
	foreign := mustServer(t, orgB, prjB)
	for name, p := range map[string]session.Principal{
		"member of A": {Source: session.SourceLocal, UserID: uuid.New(), ActiveOrgID: orgA, Scopes: []string{"*"}},
		"key of A":    {Source: session.SourceAPIKey, ActiveOrgID: orgA, AllProjects: true},
	} {
		t.Run(name, func(t *testing.T) {
			store := storeWith(foreign)
			cp := mcpserver.NewControlPlane(mcpserver.NewService(store, fakes.Transactor{}, nil).WithSealer(testSealer(t)), refs)
			ctx := session.PrincipalInto(context.Background(), p)
			for _, id := range []string{foreign.PublicID, foreign.ID.String()} {
				_, err := cp.GetMcpServer(ctx, connect.NewRequest(&mcpserverv1.GetMcpServerRequest{Id: id}))
				assert.True(t, mcpserver.IsNotFoundError(err), "Get %s: %v", id, err)
				_, err = cp.UpdateMcpServer(ctx, connect.NewRequest(&mcpserverv1.UpdateMcpServerRequest{
					Id: id, Url: "https://evil.example.com/mcp", AuthKind: mcpserverv1.McpServerAuthKind_MCP_SERVER_AUTH_KIND_NONE, TimeoutMs: 5000,
				}))
				assert.True(t, mcpserver.IsNotFoundError(err), "Update %s: %v", id, err)
				_, err = cp.DeleteMcpServer(ctx, connect.NewRequest(&mcpserverv1.DeleteMcpServerRequest{Id: id}))
				assert.True(t, mcpserver.IsNotFoundError(err), "Delete %s: %v", id, err)
				_, err = cp.ListMcpServerTools(ctx, connect.NewRequest(&mcpserverv1.ListMcpServerToolsRequest{Id: id}))
				assert.True(t, mcpserver.IsNotFoundError(err), "ListTools %s: %v", id, err)
			}
			require.Len(t, store.Servers, 1)
			assert.Equal(t, foreign.URL, store.Servers[foreign.ID].URL, "no write")
			assert.Empty(t, store.Locks)
			for _, req := range []*mcpserverv1.CreateMcpServerRequest{createReq(orgBPub, prjBPub), createReq(orgB.String(), prjB.String())} {
				_, err := cp.CreateMcpServer(ctx, connect.NewRequest(req))
				assert.True(t, mcpserver.IsNotFoundError(err), "Create in %s: %v", req.GetProjectId(), err)
				_, err = cp.ListMcpServers(ctx, connect.NewRequest(&mcpserverv1.ListMcpServersRequest{ProjectId: req.GetProjectId()}))
				assert.True(t, mcpserver.IsNotFoundError(err), "List in %s: %v", req.GetProjectId(), err)
			}
			assert.Len(t, store.Servers, 1)
		})
	}
}

func TestMcpServerReach_ListFiltersToReach(t *testing.T) {
	refs := fakes.NewPublicIDs()
	orgA, prj := uuid.New(), uuid.New()
	refs.Org(orgA)
	prjPub := refs.Project(prj, orgA)
	mine, other := mustServer(t, orgA, prj), mustServer(t, orgA, prj)
	foreign := mustServer(t, uuid.New(), prj)
	cp := mcpserver.NewControlPlane(mcpserver.NewService(storeWith(mine, other, foreign), fakes.Transactor{}, nil), refs)
	list := func(p session.Principal) ([]string, error) {
		ctx := context.Background()
		if p.Source != "" {
			ctx = session.PrincipalInto(ctx, p)
		}
		resp, err := cp.ListMcpServers(ctx, connect.NewRequest(&mcpserverv1.ListMcpServersRequest{ProjectId: prjPub}))
		if err != nil {
			return nil, err
		}
		var out []string
		for _, s := range resp.Msg.GetMcpServers() {
			out = append(out, s.GetId())
		}
		return out, nil
	}
	_, err := list(session.Principal{})
	assert.True(t, mcpserver.IsNotFoundError(err), "no principal reaches nothing: %v", err)
	got, err := list(session.System())
	require.NoError(t, err)
	assert.Len(t, got, 3)
	got, err = list(session.Principal{Source: session.SourceLocal, UserID: uuid.New(), ActiveOrgID: orgA})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{mine.PublicID, other.PublicID}, got)
	got, err = list(session.Principal{Source: session.SourceAPIKey, ActiveOrgID: orgA, ProjectIDs: []uuid.UUID{prj}, ResourceIDs: []uuid.UUID{mine.ID}})
	require.NoError(t, err)
	assert.Equal(t, []string{mine.PublicID}, got)
}

func TestMcpServerReach_OwnServerWorks(t *testing.T) {
	refs := fakes.NewPublicIDs()
	orgA, prj := uuid.New(), uuid.New()
	refs.Org(orgA)
	refs.Project(prj, orgA)
	s := mustServer(t, orgA, prj)
	store := storeWith(s)
	cp := mcpserver.NewControlPlane(mcpserver.NewService(store, fakes.Transactor{}, nil), refs)
	ctx := session.PrincipalInto(context.Background(), session.Principal{Source: session.SourceAPIKey, ActiveOrgID: orgA, ProjectIDs: []uuid.UUID{prj}})
	resp, err := cp.GetMcpServer(ctx, connect.NewRequest(&mcpserverv1.GetMcpServerRequest{Id: s.PublicID}))
	require.NoError(t, err)
	assert.Equal(t, s.PublicID, resp.Msg.GetMcpServer().GetId())
	_, err = cp.DeleteMcpServer(ctx, connect.NewRequest(&mcpserverv1.DeleteMcpServerRequest{Id: s.PublicID}))
	require.NoError(t, err)
	assert.Empty(t, store.Servers)
}
