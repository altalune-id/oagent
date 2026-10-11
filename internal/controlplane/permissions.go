package controlplane

import (
	agentv1connect "altalune.id/oagent/gen/go/agent/v1/agentv1connect"
	apikeyv1connect "altalune.id/oagent/gen/go/apikey/v1/apikeyv1connect"
	authv1connect "altalune.id/oagent/gen/go/auth/v1/authv1connect"
	orgv1connect "altalune.id/oagent/gen/go/org/v1/orgv1connect"
	projectv1connect "altalune.id/oagent/gen/go/project/v1/projectv1connect"
	webhookv1connect "altalune.id/oagent/gen/go/webhook/v1/webhookv1connect"
	"altalune.id/oagent/internal/kernel/authn"
	"altalune.id/oagent/internal/kernel/authz"
	"altalune.id/oagent/scope"
)

// PermissionTable maps every /api procedure to the permission it needs. SECURITY: a procedure missing here is denied.
func PermissionTable() authn.PermissionTable {
	return authn.PermissionTable{
		orgv1connect.OrgServiceCreateOrgProcedure: scope.OrgWrite,
		orgv1connect.OrgServiceGetOrgProcedure:    scope.OrgRead,
		orgv1connect.OrgServiceListOrgsProcedure:  scope.OrgRead,
		orgv1connect.OrgServiceUpdateOrgProcedure: scope.OrgWrite,

		projectv1connect.ProjectServiceCreateProjectProcedure: scope.ProjectWrite,
		projectv1connect.ProjectServiceGetProjectProcedure:    scope.ProjectRead,
		projectv1connect.ProjectServiceListProjectsProcedure:  scope.ProjectRead,
		projectv1connect.ProjectServiceUpdateProjectProcedure: scope.ProjectWrite,

		agentv1connect.AgentServiceCreateAgentProcedure: scope.AgentWrite,
		agentv1connect.AgentServiceGetAgentProcedure:    scope.AgentRead,
		agentv1connect.AgentServiceListAgentsProcedure:  scope.AgentRead,
		agentv1connect.AgentServiceUpdateAgentProcedure: scope.AgentWrite,
		agentv1connect.AgentServiceDeleteAgentProcedure: scope.AgentWrite,

		apikeyv1connect.APIKeyServiceListAPIKeysProcedure:  scope.ApikeyRead,
		apikeyv1connect.APIKeyServiceCreateAPIKeyProcedure: authz.AnyAuthenticated,
		apikeyv1connect.APIKeyServiceRevokeAPIKeyProcedure: authz.AnyAuthenticated,

		webhookv1connect.WebhookServiceGetWebhookProcedure:                   scope.WebhookRead,
		webhookv1connect.WebhookServiceListWebhooksProcedure:                 scope.WebhookRead,
		webhookv1connect.WebhookServiceListWebhookDeliveriesProcedure:        scope.WebhookRead,
		webhookv1connect.WebhookServiceListWebhookDeliveryAttemptsProcedure:  scope.WebhookRead,
		webhookv1connect.WebhookServiceCreateWebhookProcedure:                scope.WebhookWrite,
		webhookv1connect.WebhookServiceUpdateWebhookProcedure:                scope.WebhookWrite,
		webhookv1connect.WebhookServiceDeleteWebhookProcedure:                scope.WebhookWrite,
		webhookv1connect.WebhookServiceRotateWebhookSecretProcedure:          scope.WebhookWrite,
		webhookv1connect.WebhookServicePingWebhookProcedure:                  scope.WebhookWrite,
		webhookv1connect.WebhookServiceRetryWebhookDeliveryProcedure:         scope.WebhookWrite,
		webhookv1connect.WebhookServiceRetryFailedWebhookDeliveriesProcedure: scope.WebhookWrite,

		authv1connect.AuthServiceWhoamiProcedure: authz.AnyAuthenticated,
	}
}
