import type { Client } from '@connectrpc/connect'
import { AgentService } from '~~/gen/agent/v1/agent_pb'
import { APIKeyService } from '~~/gen/apikey/v1/apikey_pb'
import { AuthService } from '~~/gen/auth/v1/auth_pb'
import { MemberService } from '~~/gen/member/v1/member_pb'
import { OrgService } from '~~/gen/org/v1/org_pb'
import { ProjectService } from '~~/gen/project/v1/project_pb'
import { WebhookService } from '~~/gen/webhook/v1/webhook_pb'

declare module '#app' {
  interface NuxtApp {
    $orgClient: Client<typeof OrgService>
    $projectClient: Client<typeof ProjectService>
    $agentClient: Client<typeof AgentService>
    $apiKeyClient: Client<typeof APIKeyService>
    $webhookClient: Client<typeof WebhookService>
    $memberClient: Client<typeof MemberService>
    $authClient: Client<typeof AuthService>
  }
}
