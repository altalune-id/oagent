import type { Client } from '@connectrpc/connect';
import type { AgentService } from '#gen/agent/v1/agent_pb';
import type { APIKeyService } from '#gen/apikey/v1/apikey_pb';
import type { ApprovalService } from '#gen/approval/v1/approval_pb';
import type { AuthService } from '#gen/auth/v1/auth_pb';
import type { ChannelService } from '#gen/channel/v1/channel_pb';
import type { EndUserService } from '#gen/enduser/v1/enduser_pb';
import type { InviteService } from '#gen/invite/v1/invite_pb';
import type { KnowledgeService } from '#gen/knowledge/v1/knowledge_pb';
import type { McpServerService } from '#gen/mcpserver/v1/mcpserver_pb';
import type { MemberService } from '#gen/member/v1/member_pb';
import type { ModelProviderService } from '#gen/modelprovider/v1/modelprovider_pb';
import type { OrgService } from '#gen/org/v1/org_pb';
import type { ProjectService } from '#gen/project/v1/project_pb';
import type { ThreadService } from '#gen/thread/v1/thread_pb';
import type { WebhookService } from '#gen/webhook/v1/webhook_pb';

declare module '#app' {
  interface NuxtApp {
    $orgClient: Client<typeof OrgService>;
    $projectClient: Client<typeof ProjectService>;
    $agentClient: Client<typeof AgentService>;
    $modelProviderClient: Client<typeof ModelProviderService>;
    $mcpServerClient: Client<typeof McpServerService>;
    $channelClient: Client<typeof ChannelService>;
    $endUserClient: Client<typeof EndUserService>;
    $threadClient: Client<typeof ThreadService>;
    $approvalClient: Client<typeof ApprovalService>;
    $apiKeyClient: Client<typeof APIKeyService>;
    $webhookClient: Client<typeof WebhookService>;
    $memberClient: Client<typeof MemberService>;
    $inviteClient: Client<typeof InviteService>;
    $knowledgeClient: Client<typeof KnowledgeService>;
    $authClient: Client<typeof AuthService>;
  }
}
