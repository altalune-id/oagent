//go:build integration

package boot

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"

	authv1connect "altalune.id/oagent/gen/go/auth/v1/authv1connect"
	orgv1connect "altalune.id/oagent/gen/go/org/v1/orgv1connect"
	projectv1connect "altalune.id/oagent/gen/go/project/v1/projectv1connect"
	webhookv1 "altalune.id/oagent/gen/go/webhook/v1"
	webhookv1connect "altalune.id/oagent/gen/go/webhook/v1/webhookv1connect"
	"altalune.id/oagent/internal/agent"
	"altalune.id/oagent/internal/apikey"
	"altalune.id/oagent/internal/kernel/events"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/internal/permission"
	"altalune.id/oagent/internal/project"
	"altalune.id/oagent/internal/user"
	"altalune.id/oagent/scope"
)

// SECURITY: noTenantInput lists the procedures that name no org, project or resource, so another org cannot be aimed at.
var noTenantInput = map[string]string{
	authv1connect.AuthServiceWhoamiProcedure:            "describes the caller itself",
	authv1connect.AuthServiceUpdateProfileProcedure:     "changes the caller's own name; person only",
	authv1connect.AuthServiceChangePasswordProcedure:    "changes the caller's own password; person only",
	authv1connect.AuthServiceSignOutEverywhereProcedure: "ends the caller's own sessions; person only",
	orgv1connect.OrgServiceListOrgsProcedure:            "lists only the orgs the caller reaches; takes no id",
	orgv1connect.OrgServiceCreateOrgProcedure:           "makes a new org; names no existing one",
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
	org, project, otherProject, agent, webhook, delivery, member, invite, keyHint string
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
	}
	return ""
}

func seedReach(t *testing.T, f *apiFixture) reachOrg {
	t.Helper()
	ctx := t.Context()
	var s reachOrg
	a, err := org.NewOrg("org-a", "Org A", uuid.Nil)
	require.NoError(t, err)
	require.NoError(t, f.svcs.OrgStore.Save(ctx, a))
	b, err := org.NewOrg("org-b", "Org B", uuid.Nil)
	require.NoError(t, err)
	require.NoError(t, f.svcs.OrgStore.Save(ctx, b))
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
		member: u.PublicID, invite: c.Invite.PublicID, keyHint: hintOf(key),
	}
	var keyID string
	require.NoError(t, f.db.QueryRowContext(ctx, `SELECT id::text FROM api_keys WHERE org_id = $1 AND secret_hint = $2`, b.ID, hintOf(key)).Scan(&keyID))
	s.uuids = reachIDs{
		org: b.ID.String(), project: prj.ID.String(), otherProject: other.ID.String(), agent: ag.ID.String(), webhook: hook.ID.String(),
		delivery: deliveryID, member: u.ID.String(), invite: c.Invite.ID.String(), keyHint: keyID,
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
		(SELECT count(*) || md5(string_agg(url || active || updated_at || secret_primary::text, ',' ORDER BY id)) FROM webhook_endpoints WHERE org_id = $1),
		(SELECT count(*) FROM webhook_deliveries WHERE org_id = $1),
		(SELECT count(*) FROM api_keys WHERE org_id = $1 AND revoked_at IS NULL),
		(SELECT string_agg(user_id || role, ',' ORDER BY user_id) FROM memberships WHERE org_id = $1),
		(SELECT count(*) || md5(string_agg(coalesce(token_hash, '') || updated_at || coalesce(revoked_at::text, ''), ',' ORDER BY id)) FROM invites WHERE org_id = $1))`, s.orgB).Scan(&got))
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
		"delivery_id": ids.delivery, "hint": ids.keyHint, "id": ids.serviceID(method.Parent().FullName()),
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
	return slices.DeleteFunc(scope.Names(), func(n string) bool { return !scope.Mintable(n) })
}

// D7: a caller from another org reaches nothing in org B, and a new RPC fails here until it is covered.
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

// D7, Review Focus 5: a key limited to another project of org B reaches nothing outside it; it may only read its org's name and list what it reaches.
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

// Z9, Review Focus 2: a UUID or garbage in place of a public id is refused by its shape, and a well-formed unknown id is the same not-found, even for a caller who sees org B.
func TestReach_UUIDsAndGarbageAreRefused(t *testing.T) {
	f := newAPIFixture(t)
	s := seedReach(t, f)
	cookie, _, _ := f.personCookieIn(t, s.orgB, "b-owner@example.com", member.RoleOwner)
	garbage := reachIDs{"nope", "nope", "nope", "nope", "nope", "nope", "nope", "nope", "nope"}
	const unknownID = "unknwnunknwnunknwnabc"
	unknown := reachIDs{unknownID, unknownID, unknownID, unknownID, unknownID, unknownID, unknownID, unknownID, "zzzz"}
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
