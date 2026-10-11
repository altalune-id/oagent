//go:build integration

package boot

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/config"
	agentv1 "altalune.id/oagent/gen/go/agent/v1"
	agentv1connect "altalune.id/oagent/gen/go/agent/v1/agentv1connect"
	apikeyv1 "altalune.id/oagent/gen/go/apikey/v1"
	apikeyv1connect "altalune.id/oagent/gen/go/apikey/v1/apikeyv1connect"
	authv1 "altalune.id/oagent/gen/go/auth/v1"
	authv1connect "altalune.id/oagent/gen/go/auth/v1/authv1connect"
	orgv1 "altalune.id/oagent/gen/go/org/v1"
	orgv1connect "altalune.id/oagent/gen/go/org/v1/orgv1connect"
	"altalune.id/oagent/internal/apikey"
	"altalune.id/oagent/internal/kernel"
	"altalune.id/oagent/internal/kernel/authn"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/internal/project"
	"altalune.id/oagent/internal/testutil/pgtest"
	"altalune.id/oagent/internal/user"
)

type apiFixture struct {
	db     *sql.DB
	svcs   *Services
	url    string
	mcpURL string
	owner  *user.User
	orgID  string
}

func newAPIFixture(t *testing.T) *apiFixture {
	t.Helper()
	cfg := devLocalConfig()
	cfg.HTTP.StateSecret = strings.Repeat("s", 40)
	cfg.Secrets.EncryptionKey = testKeyHex
	cfg.Auth.SessionTTL = 24 * time.Hour
	cfg.Auth.Genesis = config.GenesisConfig{Email: "root@example.com", Password: "genesis-password"}
	sqlDB, schemaName := pgtest.Migrated(t)
	cfg.DB.Schema = schemaName
	svcs := buildServices(kernel.Kernel{Pool: db.Pool{W: sqlDB, R: sqlDB}, Reporter: apperror.NewReporter(nil, false)}, cfg)
	require.NoError(t, buildAuthSecrets(cfg, svcs, testLogger()))
	svcs.Caps.EnvGenesis = true
	require.NoError(t, firstBoot(t.Context(), cfg, svcs, testLogger(), false))
	required, err := installRequired(t.Context(), svcs)
	require.NoError(t, err)
	svcs.InstallRequired = required

	h := buildHTTPHandler(httpSurfaces{
		api:             buildAPIHandler(svcs, nil, cfg.API, "", testLogger()),
		mcp:             buildMCPHandler(buildMCPServer(config.MCPConfig{Enabled: true}, svcs, testLogger()), svcs, testLogger()),
		routes:          buildRoutes(cfg, svcs, nil, nil),
		session:         buildSessionMiddleware(svcs, nil),
		installRequired: required,
	})
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)

	owner, err := svcs.Users.ByEmail(t.Context(), "root@example.com")
	require.NoError(t, err)
	sys, err := svcs.Orgs.System(t.Context())
	require.NoError(t, err)
	return &apiFixture{db: sqlDB, svcs: svcs, url: ts.URL + "/api", mcpURL: ts.URL + "/mcp", owner: owner, orgID: sys.ID.String()}
}

func (f *apiFixture) mint(t *testing.T, kind apikey.Kind, scopes ...string) string {
	t.Helper()
	exp := time.Now().Add(time.Hour)
	sys, err := f.svcs.Orgs.System(t.Context())
	require.NoError(t, err)
	_, plain, err := f.svcs.APIKeys.Create(t.Context(), apikey.CreateRequest{
		OrgID: sys.ID, Kind: kind, Name: "k", Scopes: scopes, AllProjects: true, ExpiresAt: &exp, CreatedBy: f.owner.ID,
	})
	require.NoError(t, err)
	return plain
}

func withBearer[T any](req *connect.Request[T], token string) *connect.Request[T] {
	req.Header().Set("Authorization", "Bearer "+token)
	return req
}

func (f *apiFixture) whoami(t *testing.T, header, value string) (*authv1.WhoamiResponse, error) {
	t.Helper()
	req := connect.NewRequest(&authv1.WhoamiRequest{})
	if header != "" {
		req.Header().Set(header, value)
	}
	resp, err := authv1connect.NewAuthServiceClient(http.DefaultClient, f.url).Whoami(context.Background(), req)
	if err != nil {
		return nil, err
	}
	return resp.Msg, nil
}

func TestAPI_WhoamiForEveryCredential(t *testing.T) {
	f := newAPIFixture(t)

	orgKey := f.mint(t, apikey.KindOrg, "agent:read")
	got, err := f.whoami(t, "Authorization", "Bearer "+orgKey)
	require.NoError(t, err)
	assert.Equal(t, "apikey", got.GetSource())
	assert.Equal(t, []string{"agent:read"}, got.GetPermissions())
	assert.Equal(t, f.orgID, got.GetActiveOrgId())
	assert.NotEmpty(t, got.GetActiveOrgSlug())

	personal := f.mint(t, apikey.KindPersonal, "project:write", "agent:write")
	got, err = f.whoami(t, "Authorization", "Bearer "+personal)
	require.NoError(t, err)
	assert.Equal(t, f.owner.ID.String(), got.GetOwnerId())
	assert.Equal(t, "root@example.com", got.GetEmail())
	assert.ElementsMatch(t, []string{"project:write", "agent:write"}, got.GetPermissions())

	p, err := f.svcs.Auth.LoginLocal(t.Context(), authCredentials("root@example.com", "genesis-password"))
	require.NoError(t, err)
	sid, err := session.NewID()
	require.NoError(t, err)
	require.NoError(t, f.svcs.Sessions.Save(t.Context(), sid, p, time.Now().Add(time.Hour)))
	got, err = f.whoami(t, "Cookie", session.CookieName+"="+session.Sign(f.svcs.StateSecret, sid))
	require.NoError(t, err)
	assert.Equal(t, "local", got.GetSource())
	assert.True(t, got.GetOperator())

	_, err = f.whoami(t, "", "")
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))

	_, err = f.whoami(t, "Authorization", "Bearer oagent_nope")
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
}

func TestAPI_PersonalKeyFollowsOwnerRole(t *testing.T) {
	f := newAPIFixture(t)
	personal := f.mint(t, apikey.KindPersonal, "project:write", "agent:write")

	sys, err := f.svcs.Orgs.System(t.Context())
	require.NoError(t, err)
	_, err = f.db.ExecContext(t.Context(), `UPDATE memberships SET role = $1 WHERE org_id = $2 AND user_id = $3`,
		string(member.RoleMember), sys.ID, f.owner.ID)
	require.NoError(t, err)

	got, err := f.whoami(t, "Authorization", "Bearer "+personal)
	require.NoError(t, err)
	assert.Equal(t, []string{"agent:write"}, got.GetPermissions(), "demoted on the next request")

	_, err = f.db.ExecContext(t.Context(), `DELETE FROM memberships WHERE org_id = $1 AND user_id = $2`, sys.ID, f.owner.ID)
	require.NoError(t, err)
	_, err = f.whoami(t, "Authorization", "Bearer "+personal)
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err), "removed from the org")
}

func TestAPI_KeyPermissionIsChecked(t *testing.T) {
	f := newAPIFixture(t)
	reader := f.mint(t, apikey.KindOrg, "agent:read")
	sys, err := f.svcs.Orgs.System(t.Context())
	require.NoError(t, err)
	prj, err := f.svcs.Projects.BySlug(t.Context(), sys.ID, project.DefaultSlug)
	require.NoError(t, err)

	agents := agentv1connect.NewAgentServiceClient(http.DefaultClient, f.url)
	create := &agentv1.CreateAgentRequest{OrgId: sys.ID.String(), ProjectId: prj.ID.String(), Slug: "bot", Name: "Bot"}
	_, err = agents.CreateAgent(context.Background(), withBearer(connect.NewRequest(create), reader))
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))

	_, err = agents.CreateAgent(context.Background(), connect.NewRequest(create))
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err), "no credential is refused")

	_, err = orgv1connect.NewOrgServiceClient(http.DefaultClient, f.url).ListOrgs(context.Background(), connect.NewRequest(&orgv1.ListOrgsRequest{}))
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err), "no credential is refused")
}

func (f *apiFixture) ownerCookie(t *testing.T) string {
	t.Helper()
	p, err := f.svcs.Auth.LoginLocal(t.Context(), authCredentials("root@example.com", "genesis-password"))
	require.NoError(t, err)
	sid, err := session.NewID()
	require.NoError(t, err)
	require.NoError(t, f.svcs.Sessions.Save(t.Context(), sid, p, time.Now().Add(time.Hour)))
	return session.CookieName + "=" + session.Sign(f.svcs.StateSecret, sid)
}

func TestAPI_CreateRefusesAnotherOrgsProject(t *testing.T) {
	f := newAPIFixture(t)
	sys, err := f.svcs.Orgs.System(t.Context())
	require.NoError(t, err)
	other, err := org.NewOrg("other-org", "Other", uuid.Nil)
	require.NoError(t, err)
	require.NoError(t, f.svcs.OrgStore.Save(t.Context(), other))
	foreign, err := f.svcs.Projects.Create(t.Context(), project.CreateRequest{OrgID: other.ID, Slug: "theirs", Name: "Theirs"})
	require.NoError(t, err)

	_, err = agentv1connect.NewAgentServiceClient(http.DefaultClient, f.url).CreateAgent(context.Background(), withCookie(connect.NewRequest(&agentv1.CreateAgentRequest{
		OrgId: sys.ID.String(), ProjectId: foreign.ID.String(), Slug: "bot", Name: "Bot",
	}), f.ownerCookie(t)))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func (f *apiFixture) memberCookie(t *testing.T) (string, uuid.UUID) {
	t.Helper()
	ctx := t.Context()
	u, err := f.svcs.Users.Create(ctx, user.CreateRequest{Email: "member@example.com", Name: "Mem", Password: "member-password"})
	require.NoError(t, err)
	sys, err := f.svcs.Orgs.System(ctx)
	require.NoError(t, err)
	_, err = f.svcs.Members.Add(ctx, sys.ID, u.ID, member.RoleMember)
	require.NoError(t, err)
	p, err := f.svcs.Auth.LoginLocal(ctx, authCredentials("member@example.com", "member-password"))
	require.NoError(t, err)
	sid, err := session.NewID()
	require.NoError(t, err)
	require.NoError(t, f.svcs.Sessions.Save(ctx, sid, p, time.Now().Add(time.Hour)))
	return session.CookieName + "=" + session.Sign(f.svcs.StateSecret, sid), u.ID
}

func withCookie[T any](req *connect.Request[T], cookie string) *connect.Request[T] {
	req.Header().Set("Cookie", cookie)
	return req
}

// Review Focus 4: a member mints and revokes only their own personal keys, never wider than their role.
func TestAPI_MemberManagesOwnPersonalKeys(t *testing.T) {
	f := newAPIFixture(t)
	cookie, memberID := f.memberCookie(t)
	keys := apikeyv1connect.NewAPIKeyServiceClient(http.DefaultClient, f.url)
	exp := timestamppb.New(time.Now().Add(time.Hour))
	create := func(kind string, scopes ...string) (*apikeyv1.CreateAPIKeyResponse, error) {
		resp, err := keys.CreateAPIKey(context.Background(), withCookie(connect.NewRequest(&apikeyv1.CreateAPIKeyRequest{
			OrgId: f.orgID, Kind: kind, Name: "k", Scopes: scopes, AllProjects: true, ExpiresAt: exp,
		}), cookie))
		if err != nil {
			return nil, err
		}
		return resp.Msg, nil
	}

	mine, err := create("personal", "agent:read")
	require.NoError(t, err)
	assert.Equal(t, memberID.String(), f.ownerOf(t, mine.GetKey().GetId()))

	_, err = create("personal", "org:write")
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err), "above the member role")
	_, err = create("personal", "apikey:write")
	require.Error(t, err, "person-only scope")
	_, err = create("org", "agent:read")
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err), "org keys need apikey:write")

	revoke := func(id string) error {
		_, err := keys.RevokeAPIKey(context.Background(), withCookie(connect.NewRequest(&apikeyv1.RevokeAPIKeyRequest{OrgId: f.orgID, Id: id}), cookie))
		return err
	}
	orgKey := f.mint(t, apikey.KindOrg, "agent:read")
	orgKeyID := f.keyIDOf(t, orgKey)
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(revoke(orgKeyID)))
	ownersKey := f.mint(t, apikey.KindPersonal, "agent:read")
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(revoke(f.keyIDOf(t, ownersKey))))
	require.NoError(t, revoke(mine.GetKey().GetId()))

	_, err = keys.CreateAPIKey(context.Background(), withBearer(connect.NewRequest(&apikeyv1.CreateAPIKeyRequest{
		OrgId: f.orgID, Kind: "org", Name: "k", Scopes: []string{"agent:read"}, AllProjects: true, ExpiresAt: exp,
	}), orgKey))
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err), "a key never mints keys")
}

func (f *apiFixture) keyIDOf(t *testing.T, plain string) string {
	t.Helper()
	p, err := f.svcs.KeyAuthn.Authenticate(t.Context(), authn.Credential{Kind: authn.KindAPIKey, Token: plain})
	require.NoError(t, err)
	return p.KeyID.String()
}

func (f *apiFixture) ownerOf(t *testing.T, id string) string {
	t.Helper()
	var owner string
	require.NoError(t, f.db.QueryRowContext(t.Context(), `SELECT created_by::text FROM api_keys WHERE id = $1`, id).Scan(&owner))
	return owner
}
