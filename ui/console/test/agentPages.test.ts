// @vitest-environment happy-dom
import { create } from '@bufbuild/protobuf';
import { flushPromises } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { useToast } from '#base/components/ui/toast';
import { WhoamiResponseSchema } from '#gen/auth/v1/auth_pb';
import { mountPage } from '#test/mount-page';
import { deferred, setClient, setRoute } from '#test/nuxt-globals';
import ConfigurePage from '~/pages/o/[org]/projects/[project]/agents/[agent]/index.vue';
import AgentsPage from '~/pages/o/[org]/projects/[project]/agents/index.vue';
import { sampleAgents } from '~/preview/samples';
import { useOrgsStore } from '~/stores/orgs';
import { useProjectsStore } from '~/stores/projects';

const orgId = 'B6rTn1YkM8wQz3XpL5vGd';
const projectId = 'x6MH9WPWjktMwCBKHy4pF';
const support = { id: 'A7kPq2WxN4mLt8VcR3bZs', projectId, slug: 'support', name: 'Support', description: 'Answers tickets' };

function inProject(path: string, agents: unknown[], ...permissions: string[]) {
  useOrgsStore().items = [{ id: orgId, slug: 'acme', name: 'Acme', role: 'owner' }];
  useProjectsStore().items = [{ id: projectId, orgId, slug: 'web', name: 'Web' }];
  setRoute({ path: `/o/acme/projects/web${path}`, params: { org: 'acme', project: 'web', agent: 'support' } });
  setClient('$authClient', { whoami: vi.fn().mockResolvedValue(create(WhoamiResponseSchema, { source: 'local', userId: 'F5pLm2XwQ8kRt3VnZ7bYc', permissions })) });
  setClient('$agentClient', { listAgents: vi.fn().mockResolvedValue({ agents }) });
}

beforeEach(() => {
  setActivePinia(createPinia());
});

describe('agents page', () => {
  it('lists real agents in a captioned table with their description and version, and no Preview badge', async () => {
    inProject('/agents', [support], 'agent:read');
    const w = await mountPage(AgentsPage);
    expect(w.get('h1').text()).toBe('nav.agents');
    expect(w.get('caption').text()).toBe('nav.agents');
    expect(w.text()).toContain('Answers tickets');
    expect(w.find('h1 + .badge').exists()).toBe(false);
    expect(w.get('tbody .badge').text()).toBe('agent.not_published');
  });

  it('starts empty with New agent, and shows sample agents only on request, marked Preview', async () => {
    inProject('/agents', [], 'agent:read', 'agent:write');
    const w = await mountPage(AgentsPage);
    expect(w.get('h2').text()).toContain('agent.empty_title');
    const texts = () => w.findAll('button').map(b => b.text());
    expect(texts().filter(t => t.includes('agent.new'))).toHaveLength(1);
    expect(w.text()).not.toContain(sampleAgents()[0]!.name);
    await w.findAll('button').find(b => b.text() === 'agent.show_samples')!.trigger('click');
    expect(w.get('h1 + .badge').text()).toBe('preview.badge');
    expect(w.get('[role="note"]').text()).toBe('agent.samples_note');
    expect(w.get('caption').text()).toBe('agent.samples_caption');
    expect(w.text()).toContain(sampleAgents()[0]!.name);
    expect(w.text()).toContain('$3.86');
    expect(w.findAll('tbody .badge').filter(b => b.text() === 'agent.sample')).toHaveLength(sampleAgents().length);
    expect(w.findAll('li.list-row .badge').filter(b => b.text() === 'agent.sample')).toHaveLength(sampleAgents().length);
    await w.findAll('button').find(b => b.text() === 'agent.hide_samples')!.trigger('click');
    expect(w.text()).not.toContain(sampleAgents()[0]!.name);
  });

  it('offers no sample button to change agents without agent:write, but still offers samples', async () => {
    inProject('/agents', [], 'agent:read');
    const w = await mountPage(AgentsPage);
    expect(w.findAll('button').map(b => b.text())).toEqual(['agent.show_samples']);
  });
});

describe('agents page load error', () => {
  it('shows the error with a Retry that loads again', async () => {
    inProject('/agents', [], 'agent:read');
    const listAgents = vi.fn().mockRejectedValueOnce(new Error('down')).mockResolvedValue({ agents: [support] });
    setClient('$agentClient', { listAgents });
    const w = await mountPage(AgentsPage);
    const retry = w.findAll('button').find(b => b.text().includes('common.retry'))!;
    await retry.trigger('click');
    await flushPromises();
    expect(listAgents).toHaveBeenCalledTimes(2);
    expect(w.text()).toContain('Support');
  });
});

describe('deleting an agent', () => {
  it('confirms, removes the row and says so in a toast', async () => {
    inProject('/agents', [support], 'agent:read', 'agent:write');
    const deleteAgent = vi.fn().mockResolvedValue({});
    setClient('$agentClient', { listAgents: vi.fn().mockResolvedValue({ agents: [support] }), deleteAgent });
    const w = await mountPage(AgentsPage, { attachTo: document.body });
    await w.findAll('button').find(b => b.text().includes('common.delete'))!.trigger('click');
    const dialog = [...document.querySelectorAll('dialog')].find(d => d.open)!;
    [...dialog.querySelectorAll('button')].find(b => b.textContent?.includes('common.delete'))!.click();
    await flushPromises();
    expect(deleteAgent).toHaveBeenCalledWith({ id: support.id });
    expect(useToast().toasts.at(-1)).toMatchObject({ kind: 'success', text: 'agent.deleted|{"name":"Support"}' });
    w.unmount();
  });
});

const live = { instructions: 'Help.', model: 'large-2', temperature: 0.2, maxSteps: 12, maxCostCents: 10, tools: [{ name: 'orders.lookup', requireApproval: false }] };

function withDraft(publishedVersion = 12, extra: Record<string, unknown> = {}) {
  const agent = { ...support, draft: { ...live, instructions: 'Help more.' }, publishedVersion, unpublishedChanges: true, ...extra };
  const client = {
    listAgents: vi.fn().mockResolvedValue({ agents: [support] }),
    getAgent: vi.fn().mockResolvedValue({ agent }),
    listAgentVersions: vi.fn().mockResolvedValue({ versions: publishedVersion ? [{ version: publishedVersion, config: live }] : [] }),
    updateAgentDraft: vi.fn().mockImplementation(async ({ config }) => ({ agent: { ...agent, draft: config } })),
    publishAgent: vi.fn().mockResolvedValue({ agent: { ...agent, publishedVersion: publishedVersion + 1, unpublishedChanges: false }, version: { version: publishedVersion + 1, config: agent.draft }, created: true }),
  };
  setClient('$agentClient', client);
  return client;
}

describe('configure page', () => {
  it('loads the draft, says which version is live and offers Publish', async () => {
    inProject('/agents/support', [support], 'agent:read', 'agent:write');
    withDraft();
    const w = await mountPage(ConfigurePage);
    expect(w.get('h1').text()).toBe('Support');
    expect(w.get('textarea').element.value).toBe('Help more.');
    expect(w.get('[role="status"]').text()).toContain('configure.live_version|{"n":12}');
    const publish = w.findAll('button').find(b => b.text() === 'configure.publish')!;
    expect(publish.attributes('disabled')).toBeUndefined();
    expect(w.findAll('nav[aria-label="common.breadcrumb"] li').map(li => li.text())).toEqual(['nav.agents', 'Support']);
  });

  it('shows the diff before publishing and publishes the next version with a toast', async () => {
    inProject('/agents/support', [support], 'agent:read', 'agent:write');
    const client = withDraft();
    const w = await mountPage(ConfigurePage, { attachTo: document.body });
    await w.findAll('button').find(b => b.text() === 'configure.publish')!.trigger('click');
    const dialog = [...document.querySelectorAll('dialog')].find(d => d.open)!;
    expect(dialog.textContent).toContain('configure.publish_body|{"n":13,"current":12}');
    expect(dialog.textContent).toContain('configure.change_instructions');
    const pending = deferred<unknown>();
    const done = client.publishAgent.getMockImplementation()!;
    client.publishAgent.mockImplementationOnce(async (req: unknown) => {
      await pending.promise;
      return done(req);
    });
    [...dialog.querySelectorAll('button')].find(b => b.textContent?.includes('configure.publish_n'))!.click();
    await flushPromises();
    expect(dialog.textContent).toContain('configure.publishing');
    pending.resolve(undefined);
    await flushPromises();
    expect(client.publishAgent).toHaveBeenCalledWith({ id: support.id });
    expect(useToast().toasts.at(-1)).toMatchObject({ kind: 'success', text: 'configure.published|{"n":13}' });
    w.unmount();
  });

  it('edits the draft and keeps the API example in step with the form', async () => {
    inProject('/agents/support', [support], 'agent:read', 'agent:write');
    withDraft(0);
    const w = await mountPage(ConfigurePage);
    expect(w.get('[role="status"]').text()).toContain('configure.never_published');
    await w.get('select').setValue('small-3');
    expect(w.get('pre').text()).toContain('"model":"small-3"');
    expect(w.get('[role="status"]').text()).toContain('configure.unsaved');
    const toggles = w.findAll('input.toggle');
    await toggles[3]!.setValue(true);
    expect(w.get('pre').text()).toContain('knowledge.search');
    await w.findAll('input.checkbox')[3]!.setValue(true);
    expect(w.get('pre').text()).toContain('"name":"knowledge.search","requireApproval":true');
    await toggles[0]!.setValue(false);
    expect(w.get('pre').text()).not.toContain('orders.lookup');
    await w.get('input[step="0.01"]').setValue('0.5');
    expect(w.get('pre').text()).toContain('"maxCostCents":50');
    await w.get('input[value="mcp"]').setValue(true);
    expect(w.get('pre').text()).toContain('"tool": "agent_update_draft"');
  });

  it('is read-only without agent:write, and lists a draft tool missing from the catalogue', async () => {
    inProject('/agents/support', [support], 'agent:read');
    withDraft(12, { draft: { ...live, tools: [{ name: 'custom.tool', requireApproval: true }] } });
    const w = await mountPage(ConfigurePage);
    expect(w.findAll('button').some(b => b.text() === 'configure.publish')).toBe(false);
    expect(w.get('textarea').attributes('disabled')).toBeDefined();
    expect(w.text()).toContain('custom.tool');
  });

  it('opens a new agent with an empty config, never sample values, and names each approval box by its tool', async () => {
    inProject('/agents/support', [support], 'agent:read', 'agent:write');
    withDraft(0, { draft: undefined, unpublishedChanges: false });
    const w = await mountPage(ConfigurePage);
    expect(w.get('textarea').element.value).toBe('');
    expect((w.get('select').element as HTMLSelectElement).value).toBe('');
    expect(w.get('select option').text()).toBe('configure.choose_model');
    expect(w.findAll('input.toggle').map(i => (i.element as HTMLInputElement).checked)).toEqual([false, false, false, false]);
    expect(w.findAll('input.checkbox').map(i => i.attributes('aria-label'))).toEqual([
      'configure.tool_approval|{"tool":"orders.lookup"}',
      'configure.tool_approval|{"tool":"billing.charges.list"}',
      'configure.tool_approval|{"tool":"billing.refunds.create"}',
      'configure.tool_approval|{"tool":"knowledge.search"}',
    ]);
  });

  it('is a 404 for an unknown agent', async () => {
    inProject('/agents/support', [], 'agent:read');
    const errorHandler = vi.fn();
    await mountPage(ConfigurePage, { global: { config: { errorHandler } } });
    expect(errorHandler.mock.calls[0]![0]).toMatchObject({ statusCode: 404, statusMessage: 'error.agent_not_found' });
  });
});
