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
	apperrorv1 "altalune.id/oagent/gen/go/apperror/v1"
	authv1 "altalune.id/oagent/gen/go/auth/v1"
	authv1connect "altalune.id/oagent/gen/go/auth/v1/authv1connect"
	memberv1 "altalune.id/oagent/gen/go/member/v1"
	memberv1connect "altalune.id/oagent/gen/go/member/v1/memberv1connect"
	orgv1 "altalune.id/oagent/gen/go/org/v1"
	orgv1connect "altalune.id/oagent/gen/go/org/v1/orgv1connect"
	webhookv1 "altalune.id/oagent/gen/go/webhook/v1"
	webhookv1connect "altalune.id/oagent/gen/go/webhook/v1/webhookv1connect"
	"altalune.id/oagent/internal/agent"
	"altalune.id/oagent/internal/apikey"
	"altalune.id/oagent/internal/kernel"
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
	sysID  uuid.UUID
}

func newAPIFixture(t *testing.T) *apiFixture {
	t.Helper()
	cfg := devLocalConfig()
	cfg.HTTP.StateSecret = strings.Repeat("s", 40)
	cfg.Secrets.EncryptionKey = testKeyHex
	cfg.Auth.SessionTTL = 24 * time.Hour
	cfg.Auth.Genesis = config.GenesisConfig{Email: "root@example.com", Password: "genesis-password"}
	sqlDB, schemaName := pgtest.Migrated(t)
	cfg.Postgres.Schema = schemaName
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
	return &apiFixture{db: sqlDB, svcs: svcs, url: ts.URL + "/api", mcpURL: ts.URL + "/mcp", owner: owner, orgID: sys.PublicID, sysID: sys.ID}
}

func (f *apiFixture) mint(t *testing.T, kind apikey.Kind, scopes ...string) string {
	t.Helper()
	sys, err := f.svcs.Orgs.System(t.Context())
	require.NoError(t, err)
	return f.mintKey(t, apikey.CreateRequest{OrgID: sys.ID, Kind: kind, Scopes: scopes, AllProjects: true, CreatedBy: f.owner.ID})
}

func (f *apiFixture) mintKey(t *testing.T, req apikey.CreateRequest) string {
	t.Helper()
	exp := time.Now().Add(time.Hour)
	req.Name, req.ExpiresAt = "k", &exp
	_, plain, err := f.svcs.APIKeys.Create(t.Context(), req)
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
	assert.Equal(t, f.owner.PublicID, got.GetOwnerId())
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
	create := &agentv1.CreateAgentRequest{OrgId: sys.PublicID, ProjectId: prj.PublicID, Slug: "bot", Name: "Bot"}
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
		OrgId: sys.PublicID, ProjectId: foreign.PublicID, Slug: "bot", Name: "Bot",
	}), f.ownerCookie(t)))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func (f *apiFixture) memberCookie(t *testing.T) (string, uuid.UUID) {
	t.Helper()
	ctx := t.Context()
	u, err := f.svcs.Users.Create(ctx, user.CreateRequest{Email: "member@example.com", Name: "Mem", Password: "quiet-forest-lamp-9"})
	require.NoError(t, err)
	sys, err := f.svcs.Orgs.System(ctx)
	require.NoError(t, err)
	_, err = f.svcs.Members.Add(ctx, sys.ID, u.ID, member.RoleMember)
	require.NoError(t, err)
	p, err := f.svcs.Auth.LoginLocal(ctx, authCredentials("member@example.com", "quiet-forest-lamp-9"))
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
	assert.Equal(t, memberID.String(), f.ownerOf(t, mine.GetKey().GetSecretHint()))

	_, err = create("personal", "org:write")
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err), "above the member role")
	_, err = create("personal", "apikey:write")
	require.Error(t, err, "person-only scope")
	_, err = create("org", "agent:read")
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err), "org keys need apikey:write")

	revoke := func(hint string) error {
		_, err := keys.RevokeAPIKey(context.Background(), withCookie(connect.NewRequest(&apikeyv1.RevokeAPIKeyRequest{OrgId: f.orgID, Hint: hint}), cookie))
		return err
	}
	orgKey := f.mint(t, apikey.KindOrg, "agent:read")
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(revoke(hintOf(orgKey))))
	ownersKey := f.mint(t, apikey.KindPersonal, "agent:read")
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(revoke(hintOf(ownersKey))))
	require.NoError(t, revoke(mine.GetKey().GetSecretHint()))

	_, err = keys.CreateAPIKey(context.Background(), withBearer(connect.NewRequest(&apikeyv1.CreateAPIKeyRequest{
		OrgId: f.orgID, Kind: "org", Name: "k", Scopes: []string{"agent:read"}, AllProjects: true, ExpiresAt: exp,
	}), orgKey))
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err), "a key never mints keys")
}

func hintOf(plain string) string { return plain[len(plain)-4:] }

func (f *apiFixture) ownerOf(t *testing.T, hint string) string {
	t.Helper()
	var owner string
	require.NoError(t, f.db.QueryRowContext(t.Context(),
		`SELECT created_by::text FROM api_keys WHERE org_id = $1 AND secret_hint = $2 AND revoked_at IS NULL`, f.sysID, hint).Scan(&owner))
	return owner
}

func (f *apiFixture) userPublicID(t *testing.T, id uuid.UUID) string {
	t.Helper()
	u, err := f.svcs.Users.ByID(t.Context(), id)
	require.NoError(t, err)
	return u.PublicID
}

func TestAPI_WebhookBadSchemeIsInvalidArgument(t *testing.T) {
	f := newAPIFixture(t)
	p, err := f.svcs.Projects.Create(t.Context(), project.CreateRequest{OrgID: f.sysID, Slug: "hooks", Name: "Hooks"})
	require.NoError(t, err)

	_, err = webhookv1connect.NewWebhookServiceClient(http.DefaultClient, f.url).CreateWebhook(context.Background(), withCookie(connect.NewRequest(&webhookv1.CreateWebhookRequest{
		OrgId: f.orgID, ProjectId: p.PublicID, Url: "ftp://example.com/hook", EventTypes: []string{"agent.created"},
	}), f.ownerCookie(t)))
	require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err), "%v", err)
	var cerr *connect.Error
	require.ErrorAs(t, err, &cerr)
	require.Len(t, cerr.Details(), 1)
	value, err := cerr.Details()[0].Value()
	require.NoError(t, err)
	det, ok := value.(*apperrorv1.ErrorDetail)
	require.True(t, ok)
	assert.Equal(t, "WHK002", det.GetCode())
	assert.Equal(t, "https_required", det.GetMeta()["reason"])
	assert.NotContains(t, det.GetMeta(), "underlying_error")
}

// Review Focus 3: a removed person loses access at once, and their personal key is revoked, not just inactive.
func TestAPI_RemovedMemberLosesAccess(t *testing.T) {
	f := newAPIFixture(t)
	ctx := t.Context()
	cookie, memberID := f.memberCookie(t)
	sys, err := f.svcs.Orgs.System(ctx)
	require.NoError(t, err)
	exp := time.Now().Add(time.Hour)
	key, plain, err := f.svcs.APIKeys.Create(ctx, apikey.CreateRequest{
		OrgID: sys.ID, Kind: apikey.KindPersonal, Name: "laptop", Scopes: []string{"agent:read"}, AllProjects: true, ExpiresAt: &exp, CreatedBy: memberID,
	})
	require.NoError(t, err)
	_, err = f.whoami(t, "Cookie", cookie)
	require.NoError(t, err, "signed in before the removal")
	_, err = f.whoami(t, "Authorization", "Bearer "+plain)
	require.NoError(t, err, "the key works before the removal")

	_, err = memberv1connect.NewMemberServiceClient(http.DefaultClient, f.url).RemoveMember(ctx, withCookie(connect.NewRequest(
		&memberv1.RemoveMemberRequest{OrgId: sys.PublicID, UserId: f.userPublicID(t, memberID)}), f.ownerCookie(t)))
	require.NoError(t, err)

	_, err = f.whoami(t, "Cookie", cookie)
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err), "the cookie is dead")
	_, err = f.whoami(t, "Authorization", "Bearer "+plain)
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err), "the key is dead")
	var revoked sql.NullTime
	require.NoError(t, f.db.QueryRowContext(ctx, `SELECT revoked_at FROM api_keys WHERE org_id = $1 AND id = $2`, sys.ID, key.ID).Scan(&revoked))
	assert.True(t, revoked.Valid, "the key row is revoked")
	var pending int
	require.NoError(t, f.db.QueryRowContext(ctx, `SELECT count(*) FROM mail_queue WHERE status = 'pending'`).Scan(&pending))
	assert.Equal(t, 1, pending, "the notice is queued")
}

func (f *apiFixture) personCookie(t *testing.T, email string, role member.Role) (string, session.Principal, uuid.UUID) {
	t.Helper()
	return f.personCookieIn(t, f.sysID, email, role)
}

func (f *apiFixture) personCookieIn(t *testing.T, orgID uuid.UUID, email string, role member.Role) (string, session.Principal, uuid.UUID) {
	t.Helper()
	ctx := t.Context()
	u, err := f.svcs.Users.Create(ctx, user.CreateRequest{Email: email, Name: email, Password: "quiet-forest-lamp-9"})
	require.NoError(t, err)
	_, err = f.svcs.Members.Add(ctx, orgID, u.ID, role)
	require.NoError(t, err)
	p, err := f.svcs.Auth.LoginLocal(ctx, authCredentials(email, "quiet-forest-lamp-9"))
	require.NoError(t, err)
	sid, err := session.NewID()
	require.NoError(t, err)
	require.NoError(t, f.svcs.Sessions.Save(ctx, sid, p, time.Now().Add(time.Hour)))
	return session.CookieName + "=" + session.Sign(f.svcs.StateSecret, sid), p, u.ID
}

func connectCode(t *testing.T, err error) string {
	t.Helper()
	var cerr *connect.Error
	require.ErrorAs(t, err, &cerr)
	require.NotEmpty(t, cerr.Details(), "%v", err)
	value, derr := cerr.Details()[0].Value()
	require.NoError(t, derr)
	det, ok := value.(*apperrorv1.ErrorDetail)
	require.True(t, ok)
	return det.GetCode()
}

// SECURITY: in selfhosted the install org is the system org, so its admins are operators; they must still follow their org role there.
func TestAPI_SystemOrgAdminFollowsMemberRules(t *testing.T) {
	f := newAPIFixture(t)
	ctx := t.Context()
	_, _, carol := f.personCookie(t, "carol@example.com", member.RoleOwner)
	_, _, dave := f.personCookie(t, "dave@example.com", member.RoleMember)
	bobCookie, bob, _ := f.personCookie(t, "bob@example.com", member.RoleAdmin)
	require.True(t, bob.Operator, "an admin of the system org is an operator")
	c := memberv1connect.NewMemberServiceClient(http.DefaultClient, f.url)
	setRole := func(userID uuid.UUID, role member.Role) error {
		_, err := c.SetMemberRole(ctx, withCookie(connect.NewRequest(&memberv1.SetMemberRoleRequest{
			OrgId: f.orgID, UserId: f.userPublicID(t, userID), Role: string(role),
		}), bobCookie))
		return err
	}

	assert.Equal(t, "MBR006", connectCode(t, setRole(bob.UserID, member.RoleMember)), "no self-change")
	assert.Equal(t, "MBR005", connectCode(t, setRole(f.owner.ID, member.RoleAdmin)), "an admin can't demote an owner")
	_, err := c.RemoveMember(ctx, withCookie(connect.NewRequest(&memberv1.RemoveMemberRequest{OrgId: f.orgID, UserId: f.userPublicID(t, carol)}), bobCookie))
	assert.Equal(t, "MBR005", connectCode(t, err), "an admin can't remove an owner")
	assert.Equal(t, "MBR005", connectCode(t, setRole(dave, member.RoleOwner)), "an admin can't make an owner")

	other, err := org.NewOrg("other-org", "Other", uuid.Nil)
	require.NoError(t, err)
	require.NoError(t, f.svcs.OrgStore.Save(ctx, other))
	_, err = f.svcs.Members.Add(ctx, other.ID, dave, member.RoleMember)
	require.NoError(t, err)
	_, err = c.SetMemberRole(ctx, withCookie(connect.NewRequest(&memberv1.SetMemberRoleRequest{
		OrgId: other.PublicID, UserId: f.userPublicID(t, dave), Role: string(member.RoleAdmin),
	}), bobCookie))
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err), "selfhosted has no operator support in orgs Bob is not in")

	sys := f.sysID
	for id, want := range map[uuid.UUID]member.Role{bob.UserID: member.RoleAdmin, f.owner.ID: member.RoleOwner, carol: member.RoleOwner, dave: member.RoleMember} {
		m, err := f.svcs.Members.Find(ctx, sys, id)
		require.NoError(t, err)
		assert.Equal(t, want, m.Role)
	}
}

// A key scoped to a deleted agent still lists, without the gone resource.
func TestAPI_KeyListSurvivesADeletedResource(t *testing.T) {
	f := newAPIFixture(t)
	ctx := t.Context()
	prj, err := f.svcs.Projects.BySlug(ctx, f.sysID, project.DefaultSlug)
	require.NoError(t, err)
	ag, err := f.svcs.Agents.Create(ctx, agent.CreateRequest{OrgID: f.sysID, ProjectID: prj.ID, Slug: "gone", Name: "Gone"})
	require.NoError(t, err)
	cookie := f.ownerCookie(t)
	keys := apikeyv1connect.NewAPIKeyServiceClient(http.DefaultClient, f.url)
	_, err = keys.CreateAPIKey(ctx, withCookie(connect.NewRequest(&apikeyv1.CreateAPIKeyRequest{
		OrgId: f.orgID, ProjectId: prj.PublicID, Kind: "project", Name: "scoped", Scopes: []string{"agent:read"},
		ResourceIds: []string{ag.PublicID}, ExpiresAt: timestamppb.New(time.Now().Add(time.Hour)),
	}), cookie))
	require.NoError(t, err)
	require.NoError(t, f.svcs.Agents.Delete(ctx, ag.ID))

	resp, err := keys.ListAPIKeys(ctx, withCookie(connect.NewRequest(&apikeyv1.ListAPIKeysRequest{OrgId: f.orgID}), cookie))
	require.NoError(t, err, "a deleted resource must not break the org's key list")
	require.Len(t, resp.Msg.GetKeys(), 1)
	assert.Empty(t, resp.Msg.GetKeys()[0].GetResourceIds())
}
