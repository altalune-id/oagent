package permission

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/reflect/protoreflect"

	agentv1 "altalune.id/oagent/gen/go/agent/v1"
	apikeyv1 "altalune.id/oagent/gen/go/apikey/v1"
	authv1 "altalune.id/oagent/gen/go/auth/v1"
	channelv1 "altalune.id/oagent/gen/go/channel/v1"
	enduserv1 "altalune.id/oagent/gen/go/enduser/v1"
	invitev1 "altalune.id/oagent/gen/go/invite/v1"
	mcpserverv1 "altalune.id/oagent/gen/go/mcpserver/v1"
	memberv1 "altalune.id/oagent/gen/go/member/v1"
	modelproviderv1 "altalune.id/oagent/gen/go/modelprovider/v1"
	orgv1 "altalune.id/oagent/gen/go/org/v1"
	projectv1 "altalune.id/oagent/gen/go/project/v1"
	threadv1 "altalune.id/oagent/gen/go/thread/v1"
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
		channelv1.File_channel_v1_channel_proto,
		enduserv1.File_enduser_v1_enduser_proto,
		invitev1.File_invite_v1_invite_proto,
		mcpserverv1.File_mcpserver_v1_mcpserver_proto,
		memberv1.File_member_v1_member_proto,
		modelproviderv1.File_modelprovider_v1_modelprovider_proto,
		orgv1.File_org_v1_org_proto,
		projectv1.File_project_v1_project_proto,
		threadv1.File_thread_v1_thread_proto,
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
	require.Len(t, procs, 74, "Org 4, Project 5, Agent 8, APIKey 3, Webhook 11, Auth 6, Member 4, Invite 7, ModelProvider 5, McpServer 6, Thread 6, Channel 7, EndUser 2")
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
		"/org.v1.OrgService/CreateOrg":                               "org:write",
		"/org.v1.OrgService/ListOrgs":                                "org:read",
		"/org.v1.OrgService/UpdateOrg":                               "org:write",
		"/project.v1.ProjectService/UpdateProject":                   "project:write",
		"/project.v1.ProjectService/UpdateProjectEnvironment":        "project:write",
		"/project.v1.ProjectService/ListProjects":                    "project:read",
		"/agent.v1.AgentService/DeleteAgent":                         "agent:write",
		"/agent.v1.AgentService/UpdateAgentDraft":                    "agent:write",
		"/agent.v1.AgentService/PublishAgent":                        "agent:write",
		"/agent.v1.AgentService/ListAgentVersions":                   "agent:read",
		"/agent.v1.AgentService/GetAgent":                            "agent:read",
		"/modelprovider.v1.ModelProviderService/CreateModelProvider": "provider:write",
		"/modelprovider.v1.ModelProviderService/UpdateModelProvider": "provider:write",
		"/modelprovider.v1.ModelProviderService/DeleteModelProvider": "provider:write",
		"/modelprovider.v1.ModelProviderService/GetModelProvider":    "provider:read",
		"/modelprovider.v1.ModelProviderService/ListModelProviders":  "provider:read",
		"/mcpserver.v1.McpServerService/CreateMcpServer":             "mcpserver:write",
		"/mcpserver.v1.McpServerService/UpdateMcpServer":             "mcpserver:write",
		"/mcpserver.v1.McpServerService/DeleteMcpServer":             "mcpserver:write",
		"/mcpserver.v1.McpServerService/GetMcpServer":                "mcpserver:read",
		"/mcpserver.v1.McpServerService/ListMcpServers":              "mcpserver:read",
		"/mcpserver.v1.McpServerService/ListMcpServerTools":          "mcpserver:read",
		"/channel.v1.ChannelService/CreateConnection":                "channel:write",
		"/channel.v1.ChannelService/UpdateConnection":                "channel:write",
		"/channel.v1.ChannelService/SetConnectionSecret":             "channel:write",
		"/channel.v1.ChannelService/DeleteConnection":                "channel:write",
		"/channel.v1.ChannelService/GetConnection":                   "channel:read",
		"/channel.v1.ChannelService/ListConnections":                 "channel:read",
		"/channel.v1.ChannelService/GetChannelSettings":              "channel:read",
		"/enduser.v1.EndUserService/ListEndUsers":                    "thread:read",
		"/enduser.v1.EndUserService/GetEndUser":                      "thread:read",
		"/thread.v1.ThreadService/ListThreads":                       "thread:read",
		"/thread.v1.ThreadService/GetThread":                         "thread:read",
		"/thread.v1.ThreadService/ListThreadMessages":                "thread:read",
		"/thread.v1.ThreadService/ListRuns":                          "run:read",
		"/thread.v1.ThreadService/GetRun":                            "run:read",
		"/thread.v1.ThreadService/CancelRun":                         "run:create",
		"/webhook.v1.WebhookService/ListWebhookDeliveryAttempts":     "webhook:read",
		"/webhook.v1.WebhookService/RotateWebhookSecret":             "webhook:write",
		"/webhook.v1.WebhookService/RetryFailedWebhookDeliveries":    "webhook:write",
		"/apikey.v1.APIKeyService/ListAPIKeys":                       "apikey:read",
		"/apikey.v1.APIKeyService/CreateAPIKey":                      authz.AnyAuthenticated,
		"/apikey.v1.APIKeyService/RevokeAPIKey":                      authz.AnyAuthenticated,
		"/auth.v1.AuthService/Whoami":                                authz.AnyAuthenticated,
		"/auth.v1.AuthService/UpdateProfile":                         authz.PersonOnly,
		"/auth.v1.AuthService/ChangePassword":                        authz.PersonOnly,
		"/auth.v1.AuthService/SignOutEverywhere":                     authz.PersonOnly,
		"/auth.v1.AuthService/SetActiveOrg":                          authz.PersonOnly,
		"/auth.v1.AuthService/CreateOwnOrg":                          authz.PersonOnly,
		"/member.v1.MemberService/ListMembers":                       "member:read",
		"/member.v1.MemberService/SetMemberRole":                     "member:write",
		"/member.v1.MemberService/RemoveMember":                      "member:write",
		"/member.v1.MemberService/LeaveOrg":                          authz.PersonOnly,
		"/invite.v1.InviteService/ListInvites":                       "member:read",
		"/invite.v1.InviteService/CreateInvite":                      "member:write",
		"/invite.v1.InviteService/ResendInvite":                      "member:write",
		"/invite.v1.InviteService/RevokeInvite":                      "member:write",
		"/invite.v1.InviteService/DraftInvite":                       "member:write",
		"/invite.v1.InviteService/ConfirmDraftInvite":                "member:write",
		"/invite.v1.InviteService/DiscardDraftInvite":                "member:write",
	} {
		assert.Equal(t, []string{want}, table[proc], proc)
	}
}

func TestFor(t *testing.T) {
	assert.Equal(t, []string{scope.AgentRead}, For("/agent.v1.AgentService/GetAgent"))
	assert.Nil(t, For("/nope.v1.Nope/Nothing"), "an unknown procedure has no permission, so it is denied")
}
