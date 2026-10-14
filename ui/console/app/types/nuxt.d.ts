import type { Client } from '@connectrpc/connect';
import type { AgentService } from '~~/gen/agent/v1/agent_pb';
import type { APIKeyService } from '~~/gen/apikey/v1/apikey_pb';
import type { AuthService } from '~~/gen/auth/v1/auth_pb';
import type { InviteService } from '~~/gen/invite/v1/invite_pb';
import type { MemberService } from '~~/gen/member/v1/member_pb';
import type { OrgService } from '~~/gen/org/v1/org_pb';
import type { ProjectService } from '~~/gen/project/v1/project_pb';
import type { WebhookService } from '~~/gen/webhook/v1/webhook_pb';

declare module '#app' {
  interface NuxtApp {
    $orgClient: Client<typeof OrgService>;
    $projectClient: Client<typeof ProjectService>;
    $agentClient: Client<typeof AgentService>;
    $apiKeyClient: Client<typeof APIKeyService>;
    $webhookClient: Client<typeof WebhookService>;
    $memberClient: Client<typeof MemberService>;
    $inviteClient: Client<typeof InviteService>;
    $authClient: Client<typeof AuthService>;
  }
}
