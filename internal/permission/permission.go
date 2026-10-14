// Package permission holds the one table of the permissions each Connect procedure needs, for /api and MCP alike.
package permission

import (
	agentv1connect "altalune.id/oagent/gen/go/agent/v1/agentv1connect"
	apikeyv1connect "altalune.id/oagent/gen/go/apikey/v1/apikeyv1connect"
	authv1connect "altalune.id/oagent/gen/go/auth/v1/authv1connect"
	invitev1connect "altalune.id/oagent/gen/go/invite/v1/invitev1connect"
	memberv1connect "altalune.id/oagent/gen/go/member/v1/memberv1connect"
	orgv1connect "altalune.id/oagent/gen/go/org/v1/orgv1connect"
	projectv1connect "altalune.id/oagent/gen/go/project/v1/projectv1connect"
	webhookv1connect "altalune.id/oagent/gen/go/webhook/v1/webhookv1connect"
	"altalune.id/oagent/internal/kernel/authz"
	"altalune.id/oagent/scope"
)

// Table maps every procedure to the permissions its caller must hold, all of them. SECURITY: a missing or empty entry is denied.
func Table() map[string][]string {
	return map[string][]string{
		orgv1connect.OrgServiceCreateOrgProcedure: {scope.OrgWrite},
		orgv1connect.OrgServiceGetOrgProcedure:    {scope.OrgRead},
		orgv1connect.OrgServiceListOrgsProcedure:  {scope.OrgRead},
		orgv1connect.OrgServiceUpdateOrgProcedure: {scope.OrgWrite},

		projectv1connect.ProjectServiceCreateProjectProcedure: {scope.ProjectWrite},
		projectv1connect.ProjectServiceGetProjectProcedure:    {scope.ProjectRead},
		projectv1connect.ProjectServiceListProjectsProcedure:  {scope.ProjectRead},
		projectv1connect.ProjectServiceUpdateProjectProcedure: {scope.ProjectWrite},

		agentv1connect.AgentServiceCreateAgentProcedure: {scope.AgentWrite},
		agentv1connect.AgentServiceGetAgentProcedure:    {scope.AgentRead},
		agentv1connect.AgentServiceListAgentsProcedure:  {scope.AgentRead},
		agentv1connect.AgentServiceUpdateAgentProcedure: {scope.AgentWrite},
		agentv1connect.AgentServiceDeleteAgentProcedure: {scope.AgentWrite},

		apikeyv1connect.APIKeyServiceListAPIKeysProcedure:  {scope.ApikeyRead},
		apikeyv1connect.APIKeyServiceCreateAPIKeyProcedure: {authz.AnyAuthenticated},
		apikeyv1connect.APIKeyServiceRevokeAPIKeyProcedure: {authz.AnyAuthenticated},

		webhookv1connect.WebhookServiceGetWebhookProcedure:                   {scope.WebhookRead},
		webhookv1connect.WebhookServiceListWebhooksProcedure:                 {scope.WebhookRead},
		webhookv1connect.WebhookServiceListWebhookDeliveriesProcedure:        {scope.WebhookRead},
		webhookv1connect.WebhookServiceListWebhookDeliveryAttemptsProcedure:  {scope.WebhookRead},
		webhookv1connect.WebhookServiceCreateWebhookProcedure:                {scope.WebhookWrite},
		webhookv1connect.WebhookServiceUpdateWebhookProcedure:                {scope.WebhookWrite},
		webhookv1connect.WebhookServiceDeleteWebhookProcedure:                {scope.WebhookWrite},
		webhookv1connect.WebhookServiceRotateWebhookSecretProcedure:          {scope.WebhookWrite},
		webhookv1connect.WebhookServicePingWebhookProcedure:                  {scope.WebhookWrite},
		webhookv1connect.WebhookServiceRetryWebhookDeliveryProcedure:         {scope.WebhookWrite},
		webhookv1connect.WebhookServiceRetryFailedWebhookDeliveriesProcedure: {scope.WebhookWrite},

		memberv1connect.MemberServiceListMembersProcedure:   {scope.MemberRead},
		memberv1connect.MemberServiceSetMemberRoleProcedure: {scope.MemberWrite},
		memberv1connect.MemberServiceRemoveMemberProcedure:  {scope.MemberWrite},
		memberv1connect.MemberServiceLeaveOrgProcedure:      {authz.PersonOnly},

		invitev1connect.InviteServiceListInvitesProcedure:        {scope.MemberRead},
		invitev1connect.InviteServiceCreateInviteProcedure:       {scope.MemberWrite},
		invitev1connect.InviteServiceResendInviteProcedure:       {scope.MemberWrite},
		invitev1connect.InviteServiceRevokeInviteProcedure:       {scope.MemberWrite},
		invitev1connect.InviteServiceDraftInviteProcedure:        {scope.MemberWrite},
		invitev1connect.InviteServiceConfirmDraftInviteProcedure: {scope.MemberWrite},
		invitev1connect.InviteServiceDiscardDraftInviteProcedure: {scope.MemberWrite},

		authv1connect.AuthServiceWhoamiProcedure:            {authz.AnyAuthenticated},
		authv1connect.AuthServiceUpdateProfileProcedure:     {authz.PersonOnly},
		authv1connect.AuthServiceChangePasswordProcedure:    {authz.PersonOnly},
		authv1connect.AuthServiceSignOutEverywhereProcedure: {authz.PersonOnly},
	}
}

// For returns the permissions procedure needs, or nil for an unknown procedure (which is denied).
func For(procedure string) []string { return Table()[procedure] }
