import type { Component } from 'vue';
import AgentCreateView from './AgentCreateView.vue';
import AgentListView from './AgentListView.vue';
import AuthWhoami from './AuthWhoami.vue';
import DefaultView from './DefaultView.vue';
import InviteConfirm from './InviteConfirm.vue';
import MemberList from './MemberList.vue';
import OrgCreateView from './OrgCreateView.vue';
import OrgGetView from './OrgGetView.vue';
import OrgListView from './OrgListView.vue';
import ProjectCreateView from './ProjectCreateView.vue';
import ProjectListView from './ProjectListView.vue';
import WebhookCreateView from './WebhookCreateView.vue';
import WebhookDeliveriesView from './WebhookDeliveriesView.vue';
import WebhookListView from './WebhookListView.vue';
import WebhookRetryFailedView from './WebhookRetryFailedView.vue';

const registry: Record<string, Component> = {
  org_create: OrgCreateView,
  org_get: OrgGetView,
  org_list: OrgListView,
  project_create: ProjectCreateView,
  project_list: ProjectListView,
  agent_create: AgentCreateView,
  agent_list: AgentListView,
  webhook_create: WebhookCreateView,
  webhook_list: WebhookListView,
  webhook_deliveries: WebhookDeliveriesView,
  webhook_retry_failed: WebhookRetryFailedView,
  member_list: MemberList,
  member_invite: InviteConfirm,
  auth_whoami: AuthWhoami,
};

// resolveView picks the view for the calling tool, falling back to a generic renderer.
export function resolveView(tool: string): Component {
  return registry[tool] ?? DefaultView;
}
