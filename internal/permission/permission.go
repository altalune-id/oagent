// Package permission holds the one table of the permissions each Connect procedure needs, for /api and MCP alike.
package permission

import (
	agentv1connect "altalune.id/oagent/gen/go/agent/v1/agentv1connect"
	apikeyv1connect "altalune.id/oagent/gen/go/apikey/v1/apikeyv1connect"
	approvalv1connect "altalune.id/oagent/gen/go/approval/v1/approvalv1connect"
	authv1connect "altalune.id/oagent/gen/go/auth/v1/authv1connect"
	channelv1connect "altalune.id/oagent/gen/go/channel/v1/channelv1connect"
	enduserv1connect "altalune.id/oagent/gen/go/enduser/v1/enduserv1connect"
	invitev1connect "altalune.id/oagent/gen/go/invite/v1/invitev1connect"
	mcpserverv1connect "altalune.id/oagent/gen/go/mcpserver/v1/mcpserverv1connect"
	memberv1connect "altalune.id/oagent/gen/go/member/v1/memberv1connect"
	modelproviderv1connect "altalune.id/oagent/gen/go/modelprovider/v1/modelproviderv1connect"
	orgv1connect "altalune.id/oagent/gen/go/org/v1/orgv1connect"
	projectv1connect "altalune.id/oagent/gen/go/project/v1/projectv1connect"
	threadv1connect "altalune.id/oagent/gen/go/thread/v1/threadv1connect"
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

		projectv1connect.ProjectServiceCreateProjectProcedure:            {scope.ProjectWrite},
		projectv1connect.ProjectServiceGetProjectProcedure:               {scope.ProjectRead},
		projectv1connect.ProjectServiceListProjectsProcedure:             {scope.ProjectRead},
		projectv1connect.ProjectServiceUpdateProjectProcedure:            {scope.ProjectWrite},
		projectv1connect.ProjectServiceUpdateProjectEnvironmentProcedure: {scope.ProjectWrite},

		agentv1connect.AgentServiceCreateAgentProcedure:       {scope.AgentWrite},
		agentv1connect.AgentServiceGetAgentProcedure:          {scope.AgentRead},
		agentv1connect.AgentServiceListAgentsProcedure:        {scope.AgentRead},
		agentv1connect.AgentServiceUpdateAgentProcedure:       {scope.AgentWrite},
		agentv1connect.AgentServiceDeleteAgentProcedure:       {scope.AgentWrite},
		agentv1connect.AgentServiceUpdateAgentDraftProcedure:  {scope.AgentWrite},
		agentv1connect.AgentServicePublishAgentProcedure:      {scope.AgentWrite},
		agentv1connect.AgentServiceListAgentVersionsProcedure: {scope.AgentRead},

		modelproviderv1connect.ModelProviderServiceCreateModelProviderProcedure: {scope.ProviderWrite},
		modelproviderv1connect.ModelProviderServiceGetModelProviderProcedure:    {scope.ProviderRead},
		modelproviderv1connect.ModelProviderServiceListModelProvidersProcedure:  {scope.ProviderRead},
		modelproviderv1connect.ModelProviderServiceUpdateModelProviderProcedure: {scope.ProviderWrite},
		modelproviderv1connect.ModelProviderServiceDeleteModelProviderProcedure: {scope.ProviderWrite},

		mcpserverv1connect.McpServerServiceCreateMcpServerProcedure:    {scope.McpserverWrite},
		mcpserverv1connect.McpServerServiceGetMcpServerProcedure:       {scope.McpserverRead},
		mcpserverv1connect.McpServerServiceListMcpServersProcedure:     {scope.McpserverRead},
		mcpserverv1connect.McpServerServiceUpdateMcpServerProcedure:    {scope.McpserverWrite},
		mcpserverv1connect.McpServerServiceDeleteMcpServerProcedure:    {scope.McpserverWrite},
		mcpserverv1connect.McpServerServiceListMcpServerToolsProcedure: {scope.McpserverRead},
		channelv1connect.ChannelServiceCreateConnectionProcedure:       {scope.ChannelWrite},
		channelv1connect.ChannelServiceGetConnectionProcedure:          {scope.ChannelRead},
		channelv1connect.ChannelServiceListConnectionsProcedure:        {scope.ChannelRead},
		channelv1connect.ChannelServiceUpdateConnectionProcedure:       {scope.ChannelWrite},
		channelv1connect.ChannelServiceSetConnectionSecretProcedure:    {scope.ChannelWrite},
		channelv1connect.ChannelServiceDeleteConnectionProcedure:       {scope.ChannelWrite},
		channelv1connect.ChannelServiceGetChannelSettingsProcedure:     {scope.ChannelRead},
		enduserv1connect.EndUserServiceListEndUsersProcedure:           {scope.ThreadRead},
		enduserv1connect.EndUserServiceGetEndUserProcedure:             {scope.ThreadRead},
		approvalv1connect.ApprovalServiceListApprovalsProcedure:        {scope.ThreadRead},
		approvalv1connect.ApprovalServiceGetApprovalProcedure:          {scope.ThreadRead},
		approvalv1connect.ApprovalServiceDecideApprovalProcedure:       {scope.ApprovalDecide},
		threadv1connect.ThreadServiceListThreadsProcedure:              {scope.ThreadRead},
		threadv1connect.ThreadServiceGetThreadProcedure:                {scope.ThreadRead},
		threadv1connect.ThreadServiceListThreadMessagesProcedure:       {scope.ThreadRead},
		threadv1connect.ThreadServiceListRunsProcedure:                 {scope.RunRead},
		threadv1connect.ThreadServiceGetRunProcedure:                   {scope.RunRead},
		threadv1connect.ThreadServiceCancelRunProcedure:                {scope.RunCreate},

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
		authv1connect.AuthServiceSetActiveOrgProcedure:      {authz.PersonOnly},
		authv1connect.AuthServiceCreateOwnOrgProcedure:      {authz.PersonOnly},
	}
}

// For returns the permissions procedure needs, or nil for an unknown procedure (which is denied).
func For(procedure string) []string { return Table()[procedure] }
