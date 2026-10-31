import type { Descriptor, Field } from '../generic/descriptor';
import { t } from '../copy';
import { roles } from './members';

const orgFields: Field[] = [
  { key: 'name', label: t('common.name') },
  { key: 'slug', label: t('common.slug'), kind: 'mono' },
  { key: 'id', label: t('common.id'), kind: 'id' },
  { key: 'role', label: t('mcp.your_role'), kind: 'badge', badges: roles, hideEmpty: true },
  { key: 'createdAt', label: t('mcp.created'), kind: 'date' },
];

const projectFields: Field[] = [
  { key: 'name', label: t('common.name') },
  { key: 'slug', label: t('common.slug'), kind: 'mono' },
  { key: 'id', label: t('common.id'), kind: 'id' },
  { key: 'createdAt', label: t('mcp.created'), kind: 'date' },
];

const agentFields: Field[] = [
  { key: 'name', label: t('common.name') },
  { key: 'slug', label: t('common.slug'), kind: 'mono' },
  { key: 'description', label: t('common.description'), hideEmpty: true },
  { key: 'id', label: t('common.id'), kind: 'id' },
  { key: 'createdAt', label: t('mcp.created'), kind: 'date' },
];

const webhookFields: Field[] = [
  { key: 'url', label: t('field.url'), kind: 'url' },
  { key: 'description', label: t('common.description'), hideEmpty: true },
  { key: 'eventTypes', label: t('field.event_types'), kind: 'list' },
  { key: 'active', label: t('tokens.status_active'), kind: 'bool' },
  { key: 'id', label: t('common.id'), kind: 'id' },
];

const deliveryStatuses = {
  pending: { label: t('mcp.delivery_pending'), tone: 'warning' },
  delivered: { label: t('mcp.delivery_delivered'), tone: 'success' },
  failed: { label: t('mcp.delivery_failed'), tone: 'error' },
} as const;

// descriptors describes each tool a generic view renders: where its data is and how each field shows.
export const descriptors: Record<string, Descriptor> = {
  org_list: {
    kind: 'list',
    title: t('org.title'),
    rows: 'orgs',
    key: 'id',
    empty: t('mcp.empty_orgs'),
    columns: [
      { key: 'name', label: t('common.name') },
      { key: 'slug', label: t('common.slug'), kind: 'mono' },
      { key: 'role', label: t('mcp.your_role'), kind: 'badge', badges: roles },
      { key: 'id', label: t('common.id'), kind: 'id' },
    ],
    refresh: { tool: 'org_list', args: [] },
  },
  org_get: { kind: 'detail', title: t('mcp.title_org'), object: 'org', fields: orgFields },
  org_create: { kind: 'result', title: t('mcp.org_created'), object: 'org', fields: orgFields },
  project_list: {
    kind: 'list',
    title: t('project.title'),
    rows: 'projects',
    key: 'id',
    empty: t('mcp.empty_projects'),
    columns: projectFields.filter(f => f.key !== 'createdAt'),
    refresh: { tool: 'project_list', args: ['orgId'] },
  },
  project_get: { kind: 'detail', title: t('mcp.title_project'), object: 'project', fields: projectFields },
  project_create: { kind: 'result', title: t('mcp.project_created'), object: 'project', fields: projectFields },
  agent_list: {
    kind: 'list',
    title: t('nav.agents'),
    rows: 'agents',
    key: 'id',
    empty: t('mcp.empty_agents'),
    columns: agentFields.filter(f => f.key !== 'createdAt'),
    refresh: { tool: 'agent_list', args: ['projectId'] },
  },
  agent_get: { kind: 'detail', title: t('mcp.title_agent'), object: 'agent', fields: agentFields },
  agent_create: { kind: 'result', title: t('mcp.agent_created'), object: 'agent', fields: agentFields },
  agent_update: { kind: 'result', title: t('mcp.agent_updated'), object: 'agent', fields: agentFields },
  agent_delete: { kind: 'result', title: t('mcp.agent_deleted'), fields: [] },
  member_list: {
    kind: 'list',
    title: t('members.title'),
    rows: 'members',
    key: 'userId',
    empty: t('mcp.empty_members'),
    columns: [
      { key: 'name', label: t('common.name') },
      { key: 'email', label: t('field.email') },
      { key: 'role', label: t('field.role'), kind: 'badge', badges: roles },
      { key: 'joinedAt', label: t('mcp.col_joined'), kind: 'date' },
    ],
    refresh: { tool: 'member_list', args: ['orgId'] },
  },
  member_invite_confirm: {
    kind: 'result',
    title: t('mcp.invite_sent'),
    object: 'invite',
    fields: [
      { key: 'email', label: t('field.email') },
      { key: 'role', label: t('field.role'), kind: 'badge', badges: roles },
      { key: 'expiresAt', label: t('mcp.col_expires'), kind: 'date' },
    ],
  },
  member_invite_discard: {
    kind: 'result',
    title: t('mcp.invite_discarded'),
    fields: [],
    note: () => t('mcp.invite_discarded_body'),
  },
  webhook_list: {
    kind: 'list',
    title: t('webhooks.title'),
    rows: 'webhooks',
    key: 'id',
    empty: t('mcp.empty_webhooks'),
    columns: webhookFields.filter(f => f.key !== 'description'),
    refresh: { tool: 'webhook_list', args: ['orgId', 'projectId'] },
  },
  webhook_create: {
    kind: 'result',
    title: t('mcp.webhook_created'),
    fields: [
      ...webhookFields.map(f => ({ ...f, key: `webhook.${f.key}` })),
      { key: 'secret', label: t('mcp.signing_secret'), kind: 'secret' },
    ],
    note: () => t('mcp.webhook_secret_once'),
  },
  webhook_deliveries: {
    kind: 'list',
    title: t('webhooks.deliveries'),
    rows: 'deliveries',
    key: 'id',
    empty: t('webhooks.no_deliveries'),
    columns: [
      { key: 'eventType', label: t('webhooks.col_event'), kind: 'mono' },
      { key: 'status', label: t('webhooks.col_status'), kind: 'badge', badges: deliveryStatuses },
      { key: 'attempt', label: t('webhooks.col_attempt') },
      { key: 'lastError', label: t('mcp.last_error') },
      { key: 'createdAt', label: t('mcp.created'), kind: 'date' },
    ],
    refresh: { tool: 'webhook_deliveries', args: ['orgId', 'projectId', 'id'] },
  },
  webhook_retry_failed: {
    kind: 'result',
    title: t('mcp.retry_title'),
    fields: [{ key: 'requeued', label: t('mcp.requeued'), format: v => String(v ?? 0) }],
  },
};
