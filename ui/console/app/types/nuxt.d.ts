import type { Client } from '@connectrpc/connect'
import { AgentService } from '~~/gen/agent/v1/agent_pb'
import { APIKeyService } from '~~/gen/apikey/v1/apikey_pb'
import { OrgService } from '~~/gen/org/v1/org_pb'
import { ProjectService } from '~~/gen/project/v1/project_pb'

declare module '#app' {
  interface NuxtApp {
    $orgClient: Client<typeof OrgService>
    $projectClient: Client<typeof ProjectService>
    $agentClient: Client<typeof AgentService>
    $apiKeyClient: Client<typeof APIKeyService>
  }
}
