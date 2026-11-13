//go:build integration

package boot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"

	authv1connect "altalune.id/oagent/gen/go/auth/v1/authv1connect"
	channelv1connect "altalune.id/oagent/gen/go/channel/v1/channelv1connect"
	orgv1connect "altalune.id/oagent/gen/go/org/v1/orgv1connect"
	projectv1connect "altalune.id/oagent/gen/go/project/v1/projectv1connect"
	webhookv1 "altalune.id/oagent/gen/go/webhook/v1"
	webhookv1connect "altalune.id/oagent/gen/go/webhook/v1/webhookv1connect"
	"altalune.id/oagent/internal/agent"
	"altalune.id/oagent/internal/apikey"
	"altalune.id/oagent/internal/approval"
	"altalune.id/oagent/internal/channel"
	"altalune.id/oagent/internal/enduser"
	"altalune.id/oagent/internal/kernel/events"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/mcpserver"
	"altalune.id/oagent/internal/modelprovider"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/internal/permission"
	"altalune.id/oagent/internal/project"
	"altalune.id/oagent/internal/run"
	"altalune.id/oagent/internal/user"
	"altalune.id/oagent/nanoid"
	"altalune.id/oagent/scope"
)

// SECURITY: noTenantInput lists the procedures that name no org, project or resource, so another org cannot be aimed at.
var noTenantInput = map[string]string{
	authv1connect.AuthServiceWhoamiProcedure:                   "describes the caller itself",
	authv1connect.AuthServiceUpdateProfileProcedure:            "changes the caller's own name; person only",
	authv1connect.AuthServiceChangePasswordProcedure:           "changes the caller's own password; person only",
	authv1connect.AuthServiceSignOutEverywhereProcedure:        "ends the caller's own sessions; person only",
	authv1connect.AuthServiceCreateOwnOrgProcedure:             "makes the caller's own new org; names no existing one; person only",
	orgv1connect.OrgServiceListOrgsProcedure:                   "lists only the orgs a person belongs to, or any other caller reaches; takes no id",
	orgv1connect.OrgServiceCreateOrgProcedure:                  "makes a new org; names no existing one",
	channelv1connect.ChannelServiceGetChannelSettingsProcedure: "server-wide defaults for a new connection; reads no tenant row",
}

// SECURITY: limitedKeyAllowed lists the procedures a key limited to other projects of the org may still call.
var limitedKeyAllowed = map[string]string{
	orgv1connect.OrgServiceGetOrgProcedure:               "reading the org's own name is allowed",
	orgv1connect.OrgServiceListOrgsProcedure:             "reading the org's own name is allowed",
	projectv1connect.ProjectServiceListProjectsProcedure: "the list holds only the projects the key reaches",
}

type reachOrg struct {
	orgA, orgB, otherProject uuid.UUID
	// SECURITY: every request aims org B's public ids; a UUID would be refused for the wrong reason.
	pub reachIDs
	// uuids holds the same objects' internal ids, which no surface may accept.
	uuids reachIDs
}

type reachIDs struct {
	org, project, otherProject, agent, webhook, delivery, member, invite, keyHint, provider, server, thread, run, connection, endUser string
	approval, source, document                                                                                                        string
}

func (ids reachIDs) serviceID(service protoreflect.FullName) string {
	switch service {
	case "org.v1.OrgService":
		return ids.org
	case "project.v1.ProjectService":
		return ids.project
	case "agent.v1.AgentService":
		return ids.agent
	case "webhook.v1.WebhookService":
		return ids.webhook
	case "modelprovider.v1.ModelProviderService":
		return ids.provider
	case "mcpserver.v1.McpServerService":
		return ids.server
	case "channel.v1.ChannelService":
		return ids.connection
	case "enduser.v1.EndUserService":
		return ids.endUser
	case "approval.v1.ApprovalService":
		return ids.approval
	case "knowledge.v1.KnowledgeService":
		return ids.source
	}
	return ""
}

func seedReach(t *testing.T, f *apiFixture) reachOrg {
	t.Helper()
	ctx := t.Context()
	var s reachOrg
	a, err := org.NewOrg("org-a", "Org A", uuid.Nil)
	require.NoError(t, err)
	require.NoError(t, f.svcs.OrgStore.Save(tenant.WithOrg(ctx, a.ID), a))
	b, err := org.NewOrg("org-b", "Org B", uuid.Nil)
	require.NoError(t, err)
	require.NoError(t, f.svcs.OrgStore.Save(tenant.WithOrg(ctx, b.ID), b))
	s.orgA, s.orgB = a.ID, b.ID

	prj, err := f.svcs.Projects.Create(ctx, project.CreateRequest{OrgID: b.ID, Slug: "main", Name: "Main"})
	require.NoError(t, err)
	other, err := f.svcs.Projects.Create(ctx, project.CreateRequest{OrgID: b.ID, Slug: "side", Name: "Side"})
	require.NoError(t, err)
	s.otherProject = other.ID
	hook, _, err := f.svcs.Webhooks.Create(tenant.Into(ctx, tenant.Context{OrgID: b.ID, ProjectID: prj.ID}),
		"https://example.com/hook", "", []events.Type{events.AgentCreated})
	require.NoError(t, err)
	ag, err := f.svcs.Agents.Create(ctx, agent.CreateRequest{OrgID: b.ID, ProjectID: prj.ID, Slug: "bot", Name: "Bot"})
	require.NoError(t, err)
	inB := tenant.Into(ctx, tenant.Context{OrgID: b.ID, ProjectID: prj.ID})
	provider, err := f.svcs.ModelProviders.Create(inB, prj.ID, modelprovider.Input{
		Name: "openrouter", Kind: modelprovider.KindOpenAICompatible, BaseURL: "https://example.com/v1",
		Models: []modelprovider.Model{{ID: "m"}},
	}, "sk-probe")
	require.NoError(t, err)
	mcpURL := startProbeMCPServer(t)
	server, err := f.svcs.MCPServers.Create(inB, prj.ID, mcpserver.Input{
		Name: "yasaku", URL: mcpURL, AuthKind: mcpserver.AuthBearer, TimeoutMS: 5000,
	}, "tok-probe")
	require.NoError(t, err)
	conn, err := f.svcs.Channels.Create(inB, prj.ID, ag.ID, channel.Input{
		BaseURL: "https://wa.example.com", OpenWAOrg: "acme", OpenWAProject: "main", DeviceID: "dev_V1StGXR8Z5jdHi6B",
	}, "owk_probe")
	require.NoError(t, err)
	th, r, who := seedThread(inB, t, f, prj.ID, ag.ID)
	apr, err := f.svcs.Approvals.Create(inB, approval.CreateInput{
		OrgID: b.ID, ProjectID: prj.ID, AgentID: ag.ID, RunID: r.ID, ThreadID: th.ID, ThreadKind: th.Kind,
		Calls:              []approval.Call{{ToolCallID: "call_1", InterruptID: "intr_1", Server: "yasaku", Tool: "record_expense", Args: []byte(`{"amount":1}`)}},
		RaisedSnapshotHash: []byte{1},
	})
	require.NoError(t, err)
	source, document := uuid.New(), uuid.New()
	sourcePub, documentPub := nanoid.MustNew(21), nanoid.MustNew(21)
	_, err = f.db.ExecContext(ctx, `INSERT INTO knowledge_sources (id, public_id, org_id, project_id, kind, backend, name, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 'upload', 'pgvector', 'Docs', 'idle', now(), now())`, source, sourcePub, b.ID, prj.ID)
	require.NoError(t, err)
	_, err = f.db.ExecContext(ctx, `INSERT INTO knowledge_documents (id, public_id, org_id, project_id, source_id, title, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, 'Doc', 'pending', now(), now())`, document, documentPub, b.ID, prj.ID, source)
	require.NoError(t, err)
	var delivery, deliveryID string
	require.NoError(t, f.db.QueryRowContext(ctx, `SELECT public_id, id::text FROM outbox_entries WHERE target = $1`, hook.ID.String()).
		Scan(&delivery, &deliveryID))
	key := f.mintKey(t, apikey.CreateRequest{OrgID: b.ID, Kind: apikey.KindOrg, Scopes: []string{scope.AgentRead}, AllProjects: true})
	u, err := f.svcs.Users.Create(ctx, user.CreateRequest{Email: "b-member@example.com", Name: "B Member", Password: "quiet-forest-lamp-9"})
	require.NoError(t, err)
	_, err = f.svcs.Members.Add(ctx, b.ID, u.ID, member.RoleAdmin)
	require.NoError(t, err)
	c, err := f.svcs.Invites.Create(ctx, member.Requester{Operator: true}, b.ID, "b-invitee@example.com", member.RoleMember)
	require.NoError(t, err)
	s.pub = reachIDs{
		org: b.PublicID, project: prj.PublicID, otherProject: other.PublicID, agent: ag.PublicID, webhook: hook.PublicID, delivery: delivery,
		member: u.PublicID, invite: c.Invite.PublicID, keyHint: hintOf(key), provider: provider.PublicID, server: server.PublicID,
		thread: th.PublicID, run: r.PublicID, connection: conn.PublicID, endUser: who.PublicID, approval: apr.PublicID,
		source: sourcePub, document: documentPub,
	}
	var keyID string
	require.NoError(t, f.db.QueryRowContext(ctx, `SELECT id::text FROM api_keys WHERE org_id = $1 AND secret_hint = $2`, b.ID, hintOf(key)).Scan(&keyID))
	s.uuids = reachIDs{
		org: b.ID.String(), project: prj.ID.String(), otherProject: other.ID.String(), agent: ag.ID.String(), webhook: hook.ID.String(),
		delivery: deliveryID, member: u.ID.String(), invite: c.Invite.ID.String(), keyHint: keyID,
		provider: provider.ID.String(), server: server.ID.String(), thread: th.ID.String(), run: r.ID.String(), connection: conn.ID.String(), endUser: who.ID.String(),
		approval: apr.ID.String(), source: source.String(), document: document.String(),
	}
	return s
}

// NOTE: the fingerprint of org B's rows shows a refused call that still changed something.
func (s reachOrg) snapshot(t *testing.T, f *apiFixture) string {
	t.Helper()
	var got string
	require.NoError(t, f.db.QueryRowContext(t.Context(), `SELECT concat_ws('|',
		(SELECT slug || name || updated_at FROM orgs WHERE id = $1),
		(SELECT count(*) || max(updated_at)::text FROM projects WHERE org_id = $1),
		(SELECT count(*) || max(updated_at)::text FROM agents WHERE org_id = $1),
		(SELECT count(*) FROM agent_versions WHERE org_id = $1),
		(SELECT count(*) || md5(string_agg(url || active || updated_at || secret_primary::text, ',' ORDER BY id)) FROM webhook_endpoints WHERE org_id = $1),
		(SELECT count(*) FROM webhook_deliveries WHERE org_id = $1),
		(SELECT count(*) FROM api_keys WHERE org_id = $1 AND revoked_at IS NULL),
		(SELECT string_agg(user_id || role, ',' ORDER BY user_id) FROM memberships WHERE org_id = $1),
		(SELECT count(*) || md5(string_agg(coalesce(token_hash, '') || updated_at || coalesce(revoked_at::text, ''), ',' ORDER BY id)) FROM invites WHERE org_id = $1),
		(SELECT count(*) || md5(string_agg(name || base_url || models::text || updated_at || api_key_sealed::text, ',' ORDER BY id)) FROM model_providers WHERE org_id = $1),
		(SELECT count(*) || md5(string_agg(url || auth_kind || coalesce(auth_sealed::text, '') || updated_at, ',' ORDER BY id)) FROM mcp_servers WHERE org_id = $1),
		(SELECT count(*) || md5(string_agg(status || base_url || device_id || signing_secret_primary_sealed::text || coalesce(signing_secret_secondary_sealed::text, '') || device_key_sealed::text || updated_at, ',' ORDER BY id)) FROM channel_connections WHERE org_id = $1),
		(SELECT count(*) || md5(string_agg(cancel_requested || coalesce(lease_owner, '') || next_seq, ',' ORDER BY id)) FROM threads WHERE org_id = $1),
		(SELECT count(*) || md5(string_agg(status || attempt || coalesce(ended_at::text, ''), ',' ORDER BY id)) FROM runs WHERE org_id = $1))`, s.orgB).Scan(&got))
	return got
}

// NOTE: the request is filled so it passes the request rules and reaches the reach check.
func (s reachOrg) request(t *testing.T, procedure string) (protoreflect.MethodDescriptor, *dynamicpb.Message, bool) {
	t.Helper()
	method, msg, aimed := s.pub.request(t, procedure)
	fillProbe(msg)
	return method, msg, aimed
}

func (ids reachIDs) request(t *testing.T, procedure string) (protoreflect.MethodDescriptor, *dynamicpb.Message, bool) {
	t.Helper()
	d, err := protoregistry.GlobalFiles.FindDescriptorByName(protoreflect.FullName(strings.ReplaceAll(strings.TrimPrefix(procedure, "/"), "/", ".")))
	require.NoError(t, err, procedure)
	method, ok := d.(protoreflect.MethodDescriptor)
	require.True(t, ok, procedure)
	msg := dynamicpb.NewMessage(method.Input())
	tenantIDs := map[protoreflect.Name]string{
		"org_id": ids.org, "project_id": ids.project, "user_id": ids.member, "invite_id": ids.invite,
		"delivery_id": ids.delivery, "hint": ids.keyHint, "agent_id": ids.agent, "id": ids.serviceID(method.Parent().FullName()),
		"thread_id": ids.thread, "run_id": ids.run, "source_id": ids.source, "document_id": ids.document, "provider_id": ids.provider,
	}
	if strings.Contains(string(method.Name()), "Document") {
		tenantIDs["id"] = ids.document
	}
	aimed := false
	fields := method.Input().Fields()
	for i := range fields.Len() {
		fd := fields.Get(i)
		if fd.Kind() != protoreflect.StringKind || fd.IsList() {
			continue
		}
		if id, ok := tenantIDs[fd.Name()]; ok && id != "" {
			msg.Set(fd, protoreflect.ValueOfString(id))
			aimed = true
		}
	}
	return method, msg, aimed
}

func (f *apiFixture) callDynamic(t *testing.T, procedure string, method protoreflect.MethodDescriptor, msg *dynamicpb.Message, header, value string) error {
	t.Helper()
	client := connect.NewClient[dynamicpb.Message, dynamicpb.Message](http.DefaultClient, f.url+procedure,
		connect.WithSchema(method),
		connect.WithResponseInitializer(func(_ connect.Spec, out any) error {
			if m, ok := out.(*dynamicpb.Message); ok {
				*m = *dynamicpb.NewMessage(method.Output())
			}
			return nil
		}))
	req := connect.NewRequest(msg)
	req.Header().Set(header, value)
	_, err := client.CallUnary(t.Context(), req)
	return err
}

// NOTE: reads run before writes, so a write that got through cannot hide a read.
func reachOrder() []string {
	table := permission.Table()
	procs := make([]string, 0, len(table))
	for proc := range table {
		procs = append(procs, proc)
	}
	reads := func(proc string) bool {
		return !slices.ContainsFunc(table[proc], func(p string) bool { return !strings.HasSuffix(p, ":read") })
	}
	endsCallerSession := func(proc string) bool { return proc == authv1connect.AuthServiceSignOutEverywhereProcedure }
	slices.SortFunc(procs, func(a, b string) int {
		if la, lb := endsCallerSession(a), endsCallerSession(b); la != lb {
			if la {
				return 1
			}
			return -1
		}
		if ra, rb := reads(a), reads(b); ra != rb {
			if ra {
				return -1
			}
			return 1
		}
		return strings.Compare(a, b)
	})
	return procs
}

func isRefused(err error) bool {
	code := connect.CodeOf(err)
	return err != nil && (code == connect.CodeNotFound || code == connect.CodePermissionDenied)
}

func mintableScopes() []string {
	return slices.DeleteFunc(scope.Names(), func(n string) bool { return !scope.Holds(n, scope.HolderAny) })
}

// A caller from another org reaches nothing in org B, and a new RPC fails here until it is covered.
func TestReach_AnotherOrgIsRefused(t *testing.T) {
	for skipped := range noTenantInput {
		require.Contains(t, permission.Table(), skipped, "noTenantInput names a procedure that does not exist")
	}
	callers := map[string]func(t *testing.T, f *apiFixture, s reachOrg) (string, string){
		"owner of org A": func(t *testing.T, f *apiFixture, s reachOrg) (string, string) {
			cookie, p, _ := f.personCookieIn(t, s.orgA, "a-owner@example.com", member.RoleOwner)
			require.Equal(t, s.orgA, p.ActiveOrgID)
			require.False(t, p.Operator, "an operator reaches every org by design; this caller must not be one")
			return "Cookie", cookie
		},
		"org A key with every scope": func(t *testing.T, f *apiFixture, s reachOrg) (string, string) {
			return "Authorization", "Bearer " + f.mintKey(t, apikey.CreateRequest{OrgID: s.orgA, Kind: apikey.KindOrg, Scopes: mintableScopes(), AllProjects: true})
		},
	}
	for name, caller := range callers {
		t.Run(name, func(t *testing.T) {
			f := newAPIFixture(t)
			s := seedReach(t, f)
			header, value := caller(t, f, s)
			before := s.snapshot(t, f)
			for _, proc := range reachOrder() {
				method, msg, aimed := s.request(t, proc)
				if _, skip := noTenantInput[proc]; skip {
					assert.False(t, aimed, "%s is in noTenantInput but takes a tenant id", proc)
					continue
				}
				require.True(t, aimed, "%s takes no tenant id: cover it here or add it to noTenantInput with a reason", proc)
				err := f.callDynamic(t, proc, method, msg, header, value)
				assert.True(t, isRefused(err), "%s reached org B: %v", proc, err)
			}
			assert.Equal(t, before, s.snapshot(t, f), "org B is unchanged")
		})
	}
}

// A key limited to another project of org B reaches nothing outside it; it may only read its org's name and list what it reaches.
func TestReach_ProjectLimitedKeyIsRefused(t *testing.T) {
	for allowed := range limitedKeyAllowed {
		require.Contains(t, permission.Table(), allowed, "limitedKeyAllowed names a procedure that does not exist")
	}
	f := newAPIFixture(t)
	s := seedReach(t, f)
	key := f.mintKey(t, apikey.CreateRequest{OrgID: s.orgB, Kind: apikey.KindOrg, Scopes: mintableScopes(), ProjectIDs: []uuid.UUID{s.otherProject}})
	before := s.snapshot(t, f)
	for _, proc := range reachOrder() {
		reason, allowed := limitedKeyAllowed[proc]
		method, msg, aimed := s.request(t, proc)
		_, skip := noTenantInput[proc]
		if skip && !allowed {
			continue
		}
		require.True(t, aimed || skip, "%s takes no tenant id: cover it here or add it to noTenantInput with a reason", proc)
		err := f.callDynamic(t, proc, method, msg, "Authorization", "Bearer "+key)
		if allowed {
			assert.NoError(t, err, "%s: %s", proc, reason)
			continue
		}
		assert.True(t, isRefused(err), "%s reached a project the key does not hold: %v", proc, err)
	}
	assert.Equal(t, before, s.snapshot(t, f), "org B is unchanged")
}

// SECURITY: org events carry every invitee's email, so a key limited to one project cannot subscribe its endpoints to them.
func TestReach_ProjectLimitedKeyCannotSubscribeToOrgEvents(t *testing.T) {
	f := newAPIFixture(t)
	s := seedReach(t, f)
	key := f.mintKey(t, apikey.CreateRequest{OrgID: s.orgB, Kind: apikey.KindOrg, Scopes: mintableScopes(), ProjectIDs: []uuid.UUID{s.otherProject}})
	client := webhookv1connect.NewWebhookServiceClient(http.DefaultClient, f.url)
	create := func(typ events.Type) error {
		_, err := client.CreateWebhook(t.Context(), withBearer(connect.NewRequest(&webhookv1.CreateWebhookRequest{
			OrgId: s.pub.org, ProjectId: s.pub.otherProject, Url: "https://example.com/side", EventTypes: []string{string(typ)},
		}), key))
		return err
	}
	for _, typ := range []events.Type{events.InviteCreated, events.MemberAdded} {
		err := create(typ)
		assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err), "%s: %v", typ, err)
	}
	require.NoError(t, create(events.AgentCreated), "the key still subscribes to its project's events")
}

// A UUID or garbage in place of a public id is refused by its shape, and a well-formed unknown id is the same not-found, even for a caller who sees org B.
func TestReach_UUIDsAndGarbageAreRefused(t *testing.T) {
	f := newAPIFixture(t)
	s := seedReach(t, f)
	cookie, _, _ := f.personCookieIn(t, s.orgB, "b-owner@example.com", member.RoleOwner)
	garbage := reachIDs{"nope", "nope", "nope", "nope", "nope", "nope", "nope", "nope", "nope", "nope", "nope", "nope", "nope", "nope", "nope", "nope",
		"nope", "nope"}
	const unknownID = "unknwnunknwnunknwnabc"
	unknown := reachIDs{unknownID, unknownID, unknownID, unknownID, unknownID, unknownID, unknownID, unknownID, "zzzz", unknownID, unknownID, unknownID,
		unknownID, unknownID, unknownID, unknownID, unknownID, unknownID}
	before := s.snapshot(t, f)
	for name, tc := range map[string]struct {
		ids  reachIDs
		want connect.Code
	}{
		"uuid":    {s.uuids, connect.CodeInvalidArgument},
		"garbage": {garbage, connect.CodeInvalidArgument},
		"unknown": {unknown, connect.CodeNotFound},
	} {
		for _, proc := range reachOrder() {
			if _, skip := noTenantInput[proc]; skip {
				continue
			}
			method, msg, aimed := tc.ids.request(t, proc)
			require.True(t, aimed, proc)
			fillProbe(msg)
			err := f.callDynamic(t, proc, method, msg, "Cookie", cookie)
			assert.Equal(t, tc.want, connect.CodeOf(err), "%s with %s ids: %v", proc, name, err)
		}
	}
	assert.Equal(t, before, s.snapshot(t, f), "org B is unchanged")
}

func startProbeMCPServer(t *testing.T) string {
	t.Helper()
	srv := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "probe", Version: "1"}, nil)
	srv.AddTool(&sdkmcp.Tool{Name: "record_expense", InputSchema: map[string]any{"type": "object"}},
		func(context.Context, *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
			return &sdkmcp.CallToolResult{}, nil
		})
	ts := httptest.NewServer(sdkmcp.NewStreamableHTTPHandler(func(*http.Request) *sdkmcp.Server { return srv },
		&sdkmcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true}))
	t.Cleanup(ts.Close)
	return ts.URL + "/mcp"
}

// NOTE: a thread with one message and a run left running, so a refused CancelRun would show in the snapshot.
func seedThread(ctx context.Context, t *testing.T, f *apiFixture, projectID, agentID uuid.UUID) (*run.Thread, *run.Run, *enduser.EndUser) {
	t.Helper()
	tc, err := tenant.From(ctx)
	require.NoError(t, err)
	who, err := f.svcs.EndUsers.Upsert(ctx, projectID, enduser.Identity{Kind: enduser.KindAPIUser, Value: "key:probe-user"})
	require.NoError(t, err)
	th, err := f.svcs.RunStore.UpsertAPIThread(ctx, projectID, agentID, "probe-thread", who.ID)
	require.NoError(t, err)
	_, err = f.svcs.RunStore.AppendMessage(ctx, th.ID, run.Message{
		Role: run.RoleUser, Body: json.RawMessage(`{"id":"m1","role":"user","content":"hi"}`), SpeakerID: &who.ID, Status: run.MessagePending,
	})
	require.NoError(t, err)
	version := uuid.New()
	_, err = f.db.ExecContext(ctx, `INSERT INTO agent_versions (id, public_id, org_id, project_id, agent_id, version, config, created_at, kind)
		VALUES ($1, $2, $3, $4, $5, 1, '{}', now(), 'published')`, version, nanoid.MustNew(21), tc.OrgID, projectID, agentID)
	require.NoError(t, err)
	claim, err := f.svcs.RunStore.Claim(ctx, th.ID, "probe-lease", time.Hour)
	require.NoError(t, err)
	require.Equal(t, run.Claimed, claim)
	r := run.New(tc.OrgID, projectID, th.ID, version, time.Now())
	_, err = f.svcs.RunStore.StartRun(ctx, "probe-lease", r)
	require.NoError(t, err)
	require.NoError(t, f.svcs.RunStore.Release(ctx, th.ID, "probe-lease"))
	return th, r, who
}
