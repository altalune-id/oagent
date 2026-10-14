package permission

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/reflect/protoreflect"

	agentv1 "altalune.id/oagent/gen/go/agent/v1"
	apikeyv1 "altalune.id/oagent/gen/go/apikey/v1"
	authv1 "altalune.id/oagent/gen/go/auth/v1"
	invitev1 "altalune.id/oagent/gen/go/invite/v1"
	memberv1 "altalune.id/oagent/gen/go/member/v1"
	orgv1 "altalune.id/oagent/gen/go/org/v1"
	projectv1 "altalune.id/oagent/gen/go/project/v1"
	webhookv1 "altalune.id/oagent/gen/go/webhook/v1"
	"altalune.id/oagent/internal/kernel/authz"
	"altalune.id/oagent/scope"
)

func allProcedures() []string {
	var out []string
	for _, f := range []protoreflect.FileDescriptor{
		agentv1.File_agent_v1_agent_proto,
		apikeyv1.File_apikey_v1_apikey_proto,
		authv1.File_auth_v1_auth_proto,
		invitev1.File_invite_v1_invite_proto,
		memberv1.File_member_v1_member_proto,
		orgv1.File_org_v1_org_proto,
		projectv1.File_project_v1_project_proto,
		webhookv1.File_webhook_v1_webhook_proto,
	} {
		svcs := f.Services()
		for i := range svcs.Len() {
			s := svcs.Get(i)
			for j := range s.Methods().Len() {
				out = append(out, "/"+string(s.FullName())+"/"+string(s.Methods().Get(j).Name()))
			}
		}
	}
	return out
}

func TestEveryRPCHasAPermission(t *testing.T) {
	procs := allProcedures()
	require.Len(t, procs, 42, "Org 4, Project 4, Agent 5, APIKey 3, Webhook 11, Auth 4, Member 4, Invite 7")
	table := Table()
	for _, proc := range procs {
		perms := table[proc]
		assert.NotEmpty(t, perms, "%s has no permission", proc)
		for _, perm := range perms {
			if perm == authz.AnyAuthenticated || perm == authz.PersonOnly {
				assert.Len(t, perms, 1, "%s: %s is only ever the single entry", proc, perm)
				continue
			}
			assert.True(t, scope.Valid(perm), "%s maps to unknown permission %q", proc, perm)
		}
	}
	assert.Len(t, table, len(procs), "the table names no procedure that does not exist")
}

func TestPermissionTable_Spec15(t *testing.T) {
	table := Table()
	for proc, want := range map[string]string{
		"/org.v1.OrgService/CreateOrg":                            "org:write",
		"/org.v1.OrgService/ListOrgs":                             "org:read",
		"/org.v1.OrgService/UpdateOrg":                            "org:write",
		"/project.v1.ProjectService/UpdateProject":                "project:write",
		"/project.v1.ProjectService/ListProjects":                 "project:read",
		"/agent.v1.AgentService/DeleteAgent":                      "agent:write",
		"/agent.v1.AgentService/GetAgent":                         "agent:read",
		"/webhook.v1.WebhookService/ListWebhookDeliveryAttempts":  "webhook:read",
		"/webhook.v1.WebhookService/RotateWebhookSecret":          "webhook:write",
		"/webhook.v1.WebhookService/RetryFailedWebhookDeliveries": "webhook:write",
		"/apikey.v1.APIKeyService/ListAPIKeys":                    "apikey:read",
		"/apikey.v1.APIKeyService/CreateAPIKey":                   authz.AnyAuthenticated,
		"/apikey.v1.APIKeyService/RevokeAPIKey":                   authz.AnyAuthenticated,
		"/auth.v1.AuthService/Whoami":                             authz.AnyAuthenticated,
		"/auth.v1.AuthService/UpdateProfile":                      authz.PersonOnly,
		"/auth.v1.AuthService/ChangePassword":                     authz.PersonOnly,
		"/auth.v1.AuthService/SignOutEverywhere":                  authz.PersonOnly,
		"/member.v1.MemberService/ListMembers":                    "member:read",
		"/member.v1.MemberService/SetMemberRole":                  "member:write",
		"/member.v1.MemberService/RemoveMember":                   "member:write",
		"/member.v1.MemberService/LeaveOrg":                       authz.PersonOnly,
		"/invite.v1.InviteService/ListInvites":                    "member:read",
		"/invite.v1.InviteService/CreateInvite":                   "member:write",
		"/invite.v1.InviteService/ResendInvite":                   "member:write",
		"/invite.v1.InviteService/RevokeInvite":                   "member:write",
		"/invite.v1.InviteService/DraftInvite":                    "member:write",
		"/invite.v1.InviteService/ConfirmDraftInvite":             "member:write",
		"/invite.v1.InviteService/DiscardDraftInvite":             "member:write",
	} {
		assert.Equal(t, []string{want}, table[proc], proc)
	}
}

func TestFor(t *testing.T) {
	assert.Equal(t, []string{scope.AgentRead}, For("/agent.v1.AgentService/GetAgent"))
	assert.Nil(t, For("/nope.v1.Nope/Nothing"), "an unknown procedure has no permission, so it is denied")
}
