package controlplane

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/emptypb"

	agentv1 "altalune.id/oagent/gen/go/agent/v1"
	agentv1connect "altalune.id/oagent/gen/go/agent/v1/agentv1connect"
	authv1 "altalune.id/oagent/gen/go/auth/v1"
	authv1connect "altalune.id/oagent/gen/go/auth/v1/authv1connect"
	orgv1 "altalune.id/oagent/gen/go/org/v1"
	orgv1connect "altalune.id/oagent/gen/go/org/v1/orgv1connect"
	"altalune.id/oagent/internal/agent"
	"altalune.id/oagent/internal/kernel/authn"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/permission"
	"altalune.id/oagent/internal/project"
	"altalune.id/oagent/internal/testutil/fakes"
	"altalune.id/oagent/internal/user"
)

type keyAuthn struct{ p session.Principal }

func (k keyAuthn) Authenticate(_ context.Context, c authn.Credential) (session.Principal, error) {
	if c.Kind != authn.KindAPIKey || c.Token != "oagent_good" {
		return session.Principal{}, &authn.UnauthorizedError{}
	}
	return k.p, nil
}

func authServer(t *testing.T, p session.Principal) string {
	t.Helper()
	return authServerWith(t, fakes.NewPublicIDs(), p)
}

func authServerWith(t *testing.T, refs *fakes.PublicIDs, p session.Principal) string {
	t.Helper()
	srv := New(Services{
		Orgs:     org.NewService(&fakes.OrgStore{}, fakes.Transactor{}, nil).WithCreation(true),
		Projects: project.NewService(&fakes.ProjectStore{}, nil),
		Agents:   agent.NewService(&fakes.AgentStore{}, fakes.Transactor{}, nil),
		Users:    user.NewService(fakes.NewUserStore(), nil),
		Refs:     refs,
	}, nil, Options{Authn: authn.InterceptorOptions{
		Chain:       authn.Chain{keyAuthn{p: p}},
		KeyPrefix:   "oagent_",
		Permissions: permission.Table(),
	}})
	ts := httptest.NewServer(srv.Handler(""))
	t.Cleanup(ts.Close)
	return ts.URL + "/api"
}

func bearer[T any](req *connect.Request[T], token string) *connect.Request[T] {
	req.Header().Set("Authorization", "Bearer "+token)
	return req
}

func TestAPI_NoCredentialStillWorks(t *testing.T) {
	url := authServer(t, session.Principal{})
	_, err := orgv1connect.NewOrgServiceClient(http.DefaultClient, url).
		CreateOrg(context.Background(), connect.NewRequest(&orgv1.CreateOrgRequest{Slug: "acme", Name: "Acme"}))
	require.NoError(t, err)
}

func TestAPI_BadCredentialIs401(t *testing.T) {
	url := authServer(t, session.Principal{})
	_, err := orgv1connect.NewOrgServiceClient(http.DefaultClient, url).
		ListOrgs(context.Background(), bearer(connect.NewRequest(&orgv1.ListOrgsRequest{}), "oagent_bad"))
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
}

func TestAPI_KeyWithoutPermissionIs403(t *testing.T) {
	p := session.Principal{Source: session.SourceAPIKey, KeyID: uuid.New(), ActiveOrgID: uuid.New(), AllProjects: true, Scopes: []string{"agent:read"}}
	refs := fakes.NewPublicIDs()
	url := authServerWith(t, refs, p)
	_, err := agentv1connect.NewAgentServiceClient(http.DefaultClient, url).CreateAgent(context.Background(), bearer(connect.NewRequest(&agentv1.CreateAgentRequest{
		OrgId: refs.Org(p.ActiveOrgID), ProjectId: refs.Project(uuid.New(), p.ActiveOrgID), Slug: "bot", Name: "Bot",
	}), "oagent_good"))
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
}

func TestAPI_WhoamiWithKey(t *testing.T) {
	p := session.Principal{Source: session.SourceAPIKey, KeyID: uuid.New(), KeyHint: "a1b2", Name: "ci", Scopes: []string{"agent:read"}}
	url := authServer(t, p)
	c := authv1connect.NewAuthServiceClient(http.DefaultClient, url)

	resp, err := c.Whoami(context.Background(), bearer(connect.NewRequest(&authv1.WhoamiRequest{}), "oagent_good"))
	require.NoError(t, err)
	assert.Equal(t, "a1b2", resp.Msg.GetKeyHint())

	_, err = c.Whoami(context.Background(), connect.NewRequest(&authv1.WhoamiRequest{}))
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err), "whoami without a credential is 401")
}

func TestTenantFromPrincipal(t *testing.T) {
	var got tenant.Context
	var gotErr error
	next := func(ctx context.Context, _ connect.AnyRequest) (connect.AnyResponse, error) {
		got, gotErr = tenant.From(ctx)
		return connect.NewResponse(&emptypb.Empty{}), nil
	}
	h := tenantFromPrincipal()(next)

	p := session.Principal{UserID: uuid.New(), ActiveOrgID: uuid.New(), ActiveProjectID: uuid.New(), Source: session.SourceLocal}
	_, err := h(session.PrincipalInto(context.Background(), p), connect.NewRequest(&emptypb.Empty{}))
	require.NoError(t, err)
	require.NoError(t, gotErr)
	assert.Equal(t, tenant.Context{OrgID: p.ActiveOrgID, ProjectID: p.ActiveProjectID, UserID: p.UserID}, got)

	_, err = h(context.Background(), connect.NewRequest(&emptypb.Empty{}))
	require.NoError(t, err)
	assert.True(t, tenant.IsMissingError(gotErr), "no principal, no tenant")
}
