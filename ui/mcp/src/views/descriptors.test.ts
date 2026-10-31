// @vitest-environment happy-dom
import type { DetailDescriptor, ListDescriptor } from '../generic/descriptor';
import { flushPromises, mount } from '@vue/test-utils';
import { describe, expect, it, vi } from 'vitest';
import { ref } from 'vue';
import DetailView from '../generic/DetailView.vue';
import ListView from '../generic/ListView.vue';
import { descriptors } from './descriptors';
import { resolveView } from './index';

function provide(callServerTool = vi.fn(), input: Record<string, unknown> = {}) {
  return { global: { provide: { mcpApp: { callServerTool }, mcpInput: ref(input) } } };
}
const list = (tool: string) => descriptors[tool] as ListDescriptor;
const detail = (tool: string) => descriptors[tool] as DetailDescriptor;

describe('tool views', () => {
  it('serves every tool from a descriptor, keeping only the views that need their own', () => {
    for (const tool of ['member_invite_confirm', 'member_invite_discard', 'org_get', 'org_create', 'org_list', 'project_get', 'project_list', 'project_create', 'agent_get', 'agent_list', 'agent_create', 'agent_update', 'agent_delete', 'member_list', 'webhook_list', 'webhook_create', 'webhook_deliveries', 'webhook_retry_failed']) {
      expect(resolveView(tool).descriptor, tool).toBeDefined();
    }
    for (const tool of ['member_invite', 'auth_whoami']) {
      expect(resolveView(tool).descriptor, tool).toBeUndefined();
    }
  });

  it('refreshes agents for the same project', async () => {
    const callServerTool = vi.fn().mockResolvedValue({ structuredContent: { agents: [] } });
    const w = mount(ListView, { props: { data: { agents: [] }, descriptor: list('agent_list') }, ...provide(callServerTool, { project_id: 'p1' }) });
    await w.findAll('button').find(b => b.text() === 'Refresh')!.trigger('click');
    await flushPromises();
    expect(callServerTool).toHaveBeenCalledWith({ name: 'agent_list', arguments: { projectId: 'p1' } });
  });

  it('lists members with role badges and refreshes for the same org', async () => {
    const members = [{ userId: 'u1', name: 'Ada', email: 'ada@acme.test', role: 'owner' }, { userId: 'u2', email: 'bo@acme.test', role: 'member' }];
    const callServerTool = vi.fn().mockResolvedValue({ structuredContent: { members } });
    const w = mount(ListView, { props: { data: { members }, descriptor: list('member_list') }, ...provide(callServerTool, { org_id: 'o1' }) });
    expect(w.findAll('tbody .badge').map(b => b.text())).toEqual(['Owner', 'Member']);
    await w.findAll('button').find(b => b.text() === 'Refresh')!.trigger('click');
    await flushPromises();
    expect(callServerTool).toHaveBeenCalledWith({ name: 'member_list', arguments: { orgId: 'o1' } });
  });

  it('lists projects, agents, webhooks and deliveries', () => {
    const projects = mount(ListView, { props: { data: { projects: [{ id: 'p1', slug: 'web', name: 'Web' }] }, descriptor: list('project_list') }, ...provide() });
    expect(projects.text()).toContain('Web');
    const agents = mount(ListView, { props: { data: { agents: [{ id: 'a1', slug: 'support', name: 'Support', description: 'Answers tickets' }] }, descriptor: list('agent_list') }, ...provide() });
    expect(agents.text()).toContain('Answers tickets');
    const hooks = mount(ListView, { props: { data: { webhooks: [{ id: 'w1', url: 'https://hooks.acme.test/x', eventTypes: ['member.added'], active: true }] }, descriptor: list('webhook_list') }, ...provide() });
    expect(hooks.text()).toContain('member.added');
    expect(hooks.text()).toContain('Yes');
    const deliveries = mount(ListView, { props: { data: { deliveries: [{ id: 'd1', eventType: 'member.added', status: 'failed', attempt: 3, lastError: 'HTTP 500' }] }, descriptor: list('webhook_deliveries') }, ...provide() });
    expect(deliveries.find('.badge').classes()).toContain('badge-error');
    expect(deliveries.text()).toContain('HTTP 500');
  });

  it('shows a new webhook\'s signing secret once, with the warning', () => {
    const w = mount(DetailView, { props: { data: { webhook: { id: 'w1', url: 'https://hooks.acme.test/x', eventTypes: ['member.added'] }, secret: 'whsec_123' }, descriptor: detail('webhook_create') }, ...provide() });
    expect(w.text()).toContain('whsec_123');
    expect(w.find('[role="note"]').text()).toBe('Store it now; OAgent won\'t show it again.');
    expect(w.text()).toContain('https://hooks.acme.test/x');
  });

  it('shows a sent or discarded invite instead of raw JSON', () => {
    const sent = mount(DetailView, { props: { data: { invite: { email: 'bo@acme.test', role: 'admin', expiresAt: '2026-11-03T00:00:00Z' } }, descriptor: detail('member_invite_confirm') }, ...provide() });
    expect(sent.find('h1').text()).toBe('Invite sent');
    expect(sent.text()).toContain('Admin');
    const dropped = mount(DetailView, { props: { data: {}, descriptor: detail('member_invite_discard') }, ...provide() });
    expect(dropped.find('h1').text()).toBe('Invite discarded');
    expect(dropped.find('[role="note"]').text()).toBe('Draft discarded. No invite was sent.');
  });

  it('shows created orgs and projects, a requeue count, and one org', () => {
    const org = { id: 'o1', slug: 'acme', name: 'Acme', role: 'owner', createdAt: '2026-10-27T00:00:00Z' };
    expect(mount(DetailView, { props: { data: { org }, descriptor: detail('org_create') }, ...provide() }).find('[role="status"]').exists()).toBe(true);
    expect(mount(DetailView, { props: { data: { org }, descriptor: detail('org_get') }, ...provide() }).text()).toContain('Owner');
    expect(mount(DetailView, { props: { data: { project: { id: 'p1', name: 'Web', slug: 'web' } }, descriptor: detail('project_create') }, ...provide() }).text()).toContain('Web');
    expect(mount(DetailView, { props: { data: { requeued: 4 }, descriptor: detail('webhook_retry_failed') }, ...provide() }).text()).toContain('4');
    expect(mount(DetailView, { props: { data: { agent: { id: 'a1', name: 'Support', slug: 'support' } }, descriptor: detail('agent_update') }, ...provide() }).find('h1').text()).toBe('Agent updated');
    expect(mount(DetailView, { props: { data: {}, descriptor: detail('agent_delete') }, ...provide() }).find('h1').text()).toBe('Agent deleted');
  });
});
