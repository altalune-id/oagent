// @vitest-environment happy-dom
import { create } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { flushPromises } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { fakeRuntimeClients } from '~~/test/fixtures/runtime';
import { useToast } from '#base/components/ui/toast';
import { WhoamiResponseSchema } from '#gen/auth/v1/auth_pb';
import { mountPage } from '#test/mount-page';
import { connectError, setClient, setKnownKeys, setRoute } from '#test/nuxt-globals';
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

let runtimeClients: ReturnType<typeof fakeRuntimeClients>;

beforeEach(() => {
  setActivePinia(createPinia());
  runtimeClients = fakeRuntimeClients();
});

describe('agents page', () => {
  it('lists real agents in a captioned table with their version and no Preview badge, leaving the description to the filter', async () => {
    inProject('/agents', [support], 'agent:read');
    const w = await mountPage(AgentsPage);
    expect(w.get('h1').text()).toBe('nav.agents');
    expect(w.get('caption').text()).toBe('nav.agents');
    expect(w.text()).not.toContain('Answers tickets', 'the design table has no description column');
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
    expect(w.get('[role="note"]').classes()).toEqual(['flex', 'items-center', 'gap-1.5', 'text-xs', 'text-base-content/70']);
    expect(w.get('caption').text()).toBe('agent.samples_caption');
    expect(w.text()).toContain(sampleAgents()[0]!.name);
    expect(w.text()).not.toContain('$3.86', 'runs, median and cost wait for the runtime');
    expect(w.findAll('tbody td.font-mono').map(td => td.text())).toEqual(sampleAgents().toSorted((a, b) => b.updatedAt.getTime() - a.updatedAt.getTime()).map(a => a.model));
    expect(w.findAll('tbody .badge').filter(b => b.text() === 'agent.sample')).toHaveLength(sampleAgents().length);
    expect(w.find('li.list-row').exists()).toBe(false, 'phones scroll the table instead of a card list, so the version badges stay');
    expect(w.get('.overflow-x-auto').classes()).not.toContain('max-sm:hidden');
    expect(w.findAll('tbody td:last-child').every(td => td.classes().includes('text-base-content/70'))).toBe(true, 'Updated is muted');
    await w.findAll('button').find(b => b.text() === 'agent.hide_samples')!.trigger('click');
    expect(w.text()).not.toContain(sampleAgents()[0]!.name);
  });

  it('offers no sample button to change agents without agent:write, but still offers samples', async () => {
    inProject('/agents', [], 'agent:read');
    const w = await mountPage(AgentsPage);
    expect(w.findAll('button').map(b => b.text())).toEqual(['agent.show_samples']);
  });
});

describe('agents table', () => {
  const triage = { ...support, id: 'Q9vLm3XpT7kRw2NcY5bZd', slug: 'refund-triage', name: 'Refund Triage', description: 'Prepares refunds', updatedAt: timestampFromDate(new Date('2026-11-01T09:00:00Z')) };
  const older = { ...support, updatedAt: timestampFromDate(new Date('2026-10-01T09:00:00Z')) };

  it('filters by name or description, and says when nothing matches', async () => {
    inProject('/agents', [older, triage], 'agent:read');
    const w = await mountPage(AgentsPage);
    const names = () => w.findAll('tbody td:first-child').map(td => td.text());
    expect(names()).toEqual(['Refund Triage', 'Support'], 'newest first');
    expect(w.findAll('th').find(th => th.text() === 'agent.col_updated')!.attributes('aria-sort')).toBe('descending', 'newest first is descending by time');
    await w.get('input[type="search"]').setValue('tickets');
    expect(names()).toEqual(['Support']);
    await w.get('input[type="search"]').setValue('nothing here');
    expect(w.text()).toContain('agent.no_match|{"query":"nothing here"}');
    expect(w.text()).not.toContain('agent.empty_title');
  });

  it('sorts by name through the header button, flipping on a second press', async () => {
    inProject('/agents', [triage, older], 'agent:read');
    const w = await mountPage(AgentsPage);
    const names = () => w.findAll('tbody td:first-child').map(td => td.text());
    const byName = w.findAll('th button').find(b => b.text() === 'agent.col_agent')!;
    await byName.trigger('click');
    expect(names()).toEqual(['Refund Triage', 'Support']);
    expect(w.findAll('th')[0]!.attributes('aria-sort')).toBe('ascending');
    await byName.trigger('click');
    expect(names()).toEqual(['Support', 'Refund Triage']);
  });

  it('shows the version in mono with a Live or Draft changes badge', async () => {
    inProject('/agents', [{ ...support, publishedVersion: 12, unpublishedChanges: true }], 'agent:read');
    const w = await mountPage(AgentsPage);
    expect(w.get('tbody td:nth-child(2) .font-mono').text()).toBe('v12');
    expect(w.get('tbody td:nth-child(2) .badge').classes()).toEqual(['badge', 'badge-sm', 'badge-outline']);
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
    await w.findAll('button').find(b => b.attributes('aria-label') === 'agent.delete_named|{"name":"Support"}')!.trigger('click');
    const dialog = [...document.querySelectorAll('dialog')].find(d => d.open)!;
    [...dialog.querySelectorAll('button')].find(b => b.textContent?.includes('common.delete'))!.click();
    await flushPromises();
    expect(deleteAgent).toHaveBeenCalledWith({ id: support.id });
    expect(useToast().toasts.at(-1)).toMatchObject({ kind: 'success', text: 'agent.deleted|{"name":"Support"}' });
    w.unmount();
  });
});

const wireConfig = (over: Record<string, unknown> = {}) => ({ instructions: '', providerId: '', model: '', helperModel: '', temperature: 0, maxSteps: 0, maxCostCents: 0, mcpServers: [] as unknown[], providerMissing: false, tools: [] as unknown[], ...over });
const live = wireConfig({ instructions: 'Help.', model: 'acme-pro', temperature: 0.2, maxSteps: 12, maxCostCents: 10, tools: [{ name: 'acme.orders.read', requireApproval: false }] });
const openrouter = 'Pv6tQm2ZxR8wKp3VbNc4K';
const yasakuServer = 'Ms2bVn7QxK4pRt9KwZc3H';
const sampleGrants = ['record_expense', 'record_income', 'record_transfer', 'delete_tx', 'list_wallets', 'search_tx', 'period_report'].map(name => ({ name, requireApproval: !['list_wallets', 'search_tx', 'period_report'].includes(name) }));
const sampleDraft = wireConfig({ instructions: 'Catat pengeluaran.', providerId: openrouter, model: 'openai/gpt-4.1-mini', maxCostCents: 10, mcpServers: [{ serverId: yasakuServer, tools: sampleGrants, missing: false }] });

function withDraft(publishedVersion = 12, extra: Record<string, unknown> = {}, liveConfig = live) {
  const agent = { ...support, draft: { ...live, instructions: 'Help more.' }, publishedVersion, unpublishedChanges: true, ...extra };
  const client = {
    listAgents: vi.fn().mockResolvedValue({ agents: [support] }),
    getAgent: vi.fn().mockResolvedValue({ agent }),
    listAgentVersions: vi.fn().mockResolvedValue({ versions: publishedVersion ? [{ version: publishedVersion, config: liveConfig }] : [] }),
    updateAgentDraft: vi.fn().mockImplementation(async ({ config }) => ({ agent: { ...agent, draft: wireConfig({ ...config, mcpServers: config.mcpServers.map((g: object) => ({ ...g, missing: false })) }) } })),
    publishAgent: vi.fn().mockResolvedValue({ agent: { ...agent, publishedVersion: publishedVersion + 1, unpublishedChanges: false }, version: { version: publishedVersion + 1, config: agent.draft }, created: true }),
  };
  setClient('$agentClient', client);
  return client;
}

describe('configure page', () => {
  beforeEach(() => {
    vi.stubGlobal('definePageMeta', vi.fn());
    vi.stubGlobal('onBeforeRouteLeave', vi.fn());
    vi.stubGlobal('onBeforeRouteUpdate', vi.fn());
  });

  const yasaku = { ...support, id: 'Y4sKu8RmT2pLq6WnX9cVb', slug: 'yasaku-keuangan', name: 'Yasaku Keuangan' };
  const button = (w: Awaited<ReturnType<typeof mountPage>>, text: string) => w.findAll('button').find(b => b.text() === text)!;
  const openDialog = () => [...document.querySelectorAll('dialog')].find(d => d.open)!;

  function asSample(draft = sampleDraft) {
    inProject('/agents/yasaku-keuangan', [yasaku], 'agent:read', 'agent:write');
    setRoute({ path: '/o/acme/projects/web/agents/yasaku-keuangan', params: { org: 'acme', project: 'web', agent: 'yasaku-keuangan' } });
    const client = withDraft(4, {}, sampleDraft);
    client.listAgents.mockResolvedValue({ agents: [yasaku] });
    client.getAgent.mockResolvedValue({ agent: { ...yasaku, draft, publishedVersion: 4, unpublishedChanges: true } });
    return client;
  }

  it('locks the form and Publish after a failed load', async () => {
    inProject('/agents/support', [support], 'agent:read', 'agent:write');
    const client = withDraft();
    client.listAgentVersions.mockRejectedValue(new Error('offline'));
    const w = await mountPage(ConfigurePage);
    expect(w.get('form fieldset').attributes('disabled')).toBeDefined();
    expect(button(w, 'configure.publish').attributes('disabled')).toBeDefined();
    expect(client.updateAgentDraft).not.toHaveBeenCalled();
  });

  it('shows Retry after a failed load, hides the state line until it loads, and reloads on Retry', async () => {
    inProject('/agents/support', [support], 'agent:read', 'agent:write');
    const client = withDraft();
    client.listAgentVersions.mockRejectedValueOnce(new Error('offline'));
    const w = await mountPage(ConfigurePage);
    const header = () => w.get('[role="status"]').element.parentElement!.textContent!.trim();
    expect(header()).toBe('');
    await button(w, 'common.retry').trigger('click');
    await flushPromises();
    expect(client.listAgentVersions).toHaveBeenCalledTimes(2);
    expect(header()).toContain('legacy.live_needs_republish|{"n":12}');
    expect(w.get('form fieldset').attributes('disabled')).toBeUndefined();
  });

  it('marks a version published before providers as needing a republish, and jumps to the model', async () => {
    inProject('/agents/support', [support], 'agent:read', 'agent:write');
    withDraft();
    const w = await mountPage(ConfigurePage, { attachTo: document.body });
    expect(w.get('h1').text()).toBe('Support');
    expect(w.get('h1').element.parentElement!.textContent).not.toContain('preview.badge');
    expect(w.get('textarea').element.value).toBe('Help more.');
    expect(w.get('[role="alert"]').text()).toContain('legacy.banner|{"n":12}');
    expect(w.text()).toContain('legacy.was_model|{"model":"acme-pro"}');
    expect(w.text()).toContain('acme.orders.read');
    await button(w, 'legacy.pick_model').trigger('click');
    await flushPromises();
    expect(document.activeElement).toBe(w.get('#configure-model select[data-field="model"]').element);
    const nav = w.get('nav[aria-label="configure.sections"]');
    expect(nav.findAll('a').map(a => a.attributes('href'))).toEqual(['#configure-instructions', '#configure-model', '#configure-tools', '#configure-web', '#configure-memory', '#configure-limits']);
    await nav.get('select').setValue('configure-limits');
    expect(document.activeElement).toBe(w.get('#configure-limits input').element);
    expect(w.get('#configure-web').text()).toContain('configure.preview_section');
    for (const control of w.findAll('#configure-instructions textarea, #configure-model select, #configure-model input, #configure-limits input')) {
      expect(control.element.closest('label')?.textContent?.trim(), control.html()).toBeTruthy();
    }
    w.unmount();
  });

  it('blocks Publish until a provider and model are chosen, then saves them without the old tools and publishes', async () => {
    inProject('/agents/support', [support], 'agent:read', 'agent:write');
    const client = withDraft();
    const w = await mountPage(ConfigurePage, { attachTo: document.body });
    expect(button(w, 'configure.publish').attributes('disabled')).toBeDefined();
    await button(w, 'configure.view_changes').trigger('click');
    const dialog = openDialog();
    const fixes = [...dialog.querySelectorAll('[role="alert"] li button')].map(b => b.textContent!.trim());
    expect(fixes).toEqual(['publish.blocker_model', 'publish.blocker_legacy_tools']);
    const publishN = [...dialog.querySelectorAll('button')].find(b => b.textContent?.includes('configure.publish_n'))!;
    expect(publishN.disabled).toBe(true);
    expect(publishN.getAttribute('aria-describedby')).toBe(dialog.querySelector('[role="alert"]')!.id);
    expect(dialog.textContent).toContain('configure.change_instructions');
    expect(dialog.textContent).toContain('configure.publish_body|{"n":13,"current":12}');
    expect(dialog.textContent).toContain('configure.preview_not_published');
    (dialog.querySelectorAll('[role="alert"] li button')[1] as HTMLButtonElement).click();
    await flushPromises();
    expect(openDialog()).toBeUndefined();
    await button(w, 'legacy.remove_old_tools').trigger('click');
    expect(w.text()).not.toContain('legacy.old_tools_hint');
    const [provider, model] = w.findAll('#configure-model select');
    await provider!.setValue(openrouter);
    expect(model!.findAll('option').map(o => o.text())).toEqual(['configure.choose_model', 'configure.model_vision|{"model":"openai/gpt-4.1-mini"}', 'configure.model_vision|{"model":"google/gemini-2.5-flash"}']);
    await model!.setValue('openai/gpt-4.1-mini');
    expect(w.get('#configure-model').text()).toContain('configure.model_price|{"input":"0.40","output":"1.60"}');
    expect(button(w, 'configure.publish').attributes('disabled')).toBeUndefined();
    await button(w, 'configure.publish').trigger('click');
    const confirm = [...openDialog().querySelectorAll('button')].find(b => b.textContent?.includes('configure.publish_n'))!;
    confirm.click();
    await flushPromises();
    const sent = client.updateAgentDraft.mock.calls.at(-1)![0].config;
    expect(sent).toMatchObject({ providerId: openrouter, model: 'openai/gpt-4.1-mini', mcpServers: [] });
    expect(sent).not.toHaveProperty('tools');
    expect(client.publishAgent).toHaveBeenCalledWith({ id: support.id });
    expect(useToast().toasts.at(-1)).toMatchObject({ kind: 'success', text: 'configure.published|{"n":13}' });
    w.unmount();
  });

  it('puts an unpriced helper model\'s cap error on the helper field, and its fix focuses the helper', async () => {
    const { useModelProvidersStore } = await import('~/stores/modelProviders');
    asSample({ ...sampleDraft, helperModel: 'free-helper' });
    const w = await mountPage(ConfigurePage, { attachTo: document.body });
    useModelProvidersStore().items[0]!.models.push({ id: 'free-helper', vision: false });
    await flushPromises();
    const helper = w.get('#configure-model select[data-field="helper"]');
    expect(helper.attributes('aria-invalid')).toBe('true');
    expect(w.get('#configure-model').text()).toContain('limits.helper_unpriced|{"model":"free-helper","provider":"OpenRouter"}');
    expect(w.get('#configure-limits').text()).not.toContain('limits.unpriced');
    await button(w, 'configure.view_changes').trigger('click');
    const fix = [...openDialog().querySelectorAll('[role="alert"] li button')].find(b => b.textContent?.includes('publish.blocker_helper_unpriced')) as HTMLButtonElement;
    fix.click();
    await flushPromises();
    expect(document.activeElement).toBe(helper.element);
    w.unmount();
  });

  it('shows the AGT006 refusal at Publish', async () => {
    setKnownKeys('apperror.AGT006');
    const client = asSample();
    client.publishAgent.mockRejectedValue(connectError('AGT006'));
    const w = await mountPage(ConfigurePage, { attachTo: document.body });
    await button(w, 'configure.publish').trigger('click');
    const dialog = openDialog();
    [...dialog.querySelectorAll('button')].find(b => b.textContent?.includes('configure.publish_n'))!.click();
    await flushPromises();
    expect(openDialog().textContent).toContain('apperror.AGT006');
    w.unmount();
  });

  it('offers Retry next to a failed autosave and clears the old-tools fix once a save drops them', async () => {
    vi.useFakeTimers();
    inProject('/agents/support', [support], 'agent:read', 'agent:write');
    const client = withDraft();
    client.updateAgentDraft.mockRejectedValueOnce(new Error('offline'));
    const w = await mountPage(ConfigurePage);
    await w.get('textarea').setValue('Help even more.');
    await vi.advanceTimersByTimeAsync(5000);
    expect(client.updateAgentDraft).toHaveBeenCalledOnce();
    expect(w.text()).toContain('error.save_draft');
    expect(w.text()).toContain('legacy.old_tools_hint');
    await button(w, 'common.retry').trigger('click');
    await flushPromises();
    expect(client.updateAgentDraft).toHaveBeenCalledTimes(2);
    expect(w.text()).not.toContain('error.save_draft');
    expect(w.text()).not.toContain('legacy.old_tools_hint');
    expect(w.text()).not.toContain('publish.blocker_legacy_tools');
    vi.useRealTimers();
  });

  it('places an AGT007 refusal under the field it names', async () => {
    vi.useFakeTimers();
    setKnownKeys('apperror.AGT007_helper_model');
    const client = asSample();
    client.updateAgentDraft.mockRejectedValue(connectError('AGT007', { reason: 'helper_model' }));
    const w = await mountPage(ConfigurePage);
    await w.findAll('#configure-model select')[2]!.setValue('google/gemini-2.5-flash');
    await vi.advanceTimersByTimeAsync(1000);
    await flushPromises();
    const helper = w.findAll('#configure-model select')[2]!;
    expect(helper.attributes('aria-invalid')).toBe('true');
    expect(w.get(`#${helper.attributes('aria-describedby')!.split(' ')[1]}`).text()).toBe('apperror.AGT007_helper_model');
    expect(w.text().match(/apperror\.AGT007_helper_model/g)).toHaveLength(1);
    vi.useRealTimers();
  });

  it('opens the sample agent with its yasaku grants and suggested approvals, listing tools lazily', async () => {
    asSample();
    const w = await mountPage(ConfigurePage, { attachTo: document.body });
    const tools = w.get('#configure-tools');
    expect(tools.get('summary .sr-only').text()).toBe('grants.collapse_label|{"server":"yasaku","on":7,"total":13}');
    expect(tools.findAll('details').map(d => (d.element as HTMLDetailsElement).open)).toEqual([false, false]);
    expect(tools.text()).toContain('mcptools.not_grantable');
    expect(runtimeClients.server.listMcpServerTools.mock.calls.map(c => c[0].id)).toEqual([yasakuServer]);
    const untrusted = tools.findAll('.collapse')[1]!;
    expect(untrusted.findAll('li input[type="checkbox"]')).toHaveLength(0);
    (untrusted.element as HTMLDetailsElement).open = true;
    untrusted.element.dispatchEvent(new Event('toggle'));
    await flushPromises();
    expect(runtimeClients.server.listMcpServerTools.mock.calls.map(c => c[0].id)).toEqual([yasakuServer, 'Ms8kWq3NvT6yHp7ZxR5cJ']);
    (untrusted.element as HTMLDetailsElement).open = false;
    untrusted.element.dispatchEvent(new Event('toggle'));
    const listBills = untrusted.findAll('li input[type="checkbox"]')[0]!;
    await listBills.setValue(true);
    const ask = untrusted.get('li input.toggle');
    expect((ask.element as HTMLInputElement).checked, 'a read-only hint on an untrusted server still asks').toBe(true);
    expect(untrusted.find('[role="alert"]').exists()).toBe(false);
    await ask.setValue(false);
    expect(untrusted.get('[role="alert"]').text()).toBe('grants.untrusted_no_ask');
    const web = w.get('#configure-web');
    await web.get('textarea').setValue('');
    expect(web.text()).toContain('webtools.allowlist_required');
    await web.get('textarea').setValue('bca.co.id\nmandiri.co.id');
    expect(web.text()).not.toContain('webtools.allowlist_required');
    expect(web.findAll('input.toggle')[1]!.attributes('disabled')).toBeDefined();
    expect(web.text()).toContain('webtools.search_needs_provider');
    const memory = w.get('#configure-memory');
    const sources = memory.findAll('li input');
    expect(sources.map(i => [i.element.checked, i.attributes('disabled') !== undefined])).toEqual([[true, false], [false, true], [false, true]]);
    await sources[0]!.setValue(false);
    const { useMcpServersStore } = await import('~/stores/mcpServers');
    useMcpServersStore().items = useMcpServersStore().items.filter(x => x.name !== 'yasaku');
    await flushPromises();
    expect(w.get('#configure-tools').text()).toContain('grants.server_gone|{"n":7}');
    await button(w, 'configure.view_changes').trigger('click');
    expect(openDialog().textContent).toContain('publish.blocker_server_missing');
    await button(w, 'grants.remove_missing').trigger('click');
    expect(w.get('#configure-tools').text()).not.toContain('grants.server_gone');
    w.unmount();
  });

  it('warns about a deleted provider and a deleted server the draft still names, and drops that grant when saving', async () => {
    vi.useFakeTimers();
    const client = asSample({ ...sampleDraft, providerId: '', providerMissing: true, mcpServers: [...sampleDraft.mcpServers, { serverId: 'GneZ23456789abcdefghj', tools: [{ name: 'x', requireApproval: true }], missing: true }] });
    const w = await mountPage(ConfigurePage);
    expect(w.get('#configure-model [role="alert"]').text()).toContain('configure.provider_missing');
    expect(w.get('#configure-tools').text()).toContain('grants.server_gone|{"n":1}');
    await w.get('textarea').setValue('Catat semuanya.');
    await vi.advanceTimersByTimeAsync(1000);
    expect(client.updateAgentDraft.mock.calls[0]![0].config.mcpServers.map((g: { serverId: string }) => g.serverId)).toEqual([yasakuServer]);
    vi.useRealTimers();
  });

  it('keeps every server list closed on a phone', async () => {
    vi.stubGlobal('matchMedia', vi.fn().mockReturnValue({ matches: false }));
    asSample();
    const w = await mountPage(ConfigurePage);
    expect(w.findAll('#configure-tools details').map(d => (d.element as HTMLDetailsElement).open)).toEqual([false, false]);
    expect(w.get('#configure-tools summary').text()).toContain('grants.on_count|{"on":7,"total":13}');
    vi.unstubAllGlobals();
  });

  it('refuses a cost cap on an unpriced model and offers no knowledge without an embedding model', async () => {
    asSample();
    const w = await mountPage(ConfigurePage);
    const [provider, model] = w.findAll('#configure-model select');
    await provider!.setValue('Pv9pHs4YvT7kMq7WcXe2B');
    expect(model!.findAll('option').map(o => o.text())[1]).toBe('configure.model_unpriced_option|{"model":"llama-3.3-70b-versatile"}');
    await model!.setValue('llama-3.3-70b-versatile');
    expect(w.get('#configure-model').text()).toContain('configure.model_unpriced');
    const cost = w.get('#configure-limits input[inputmode="decimal"]');
    expect((cost.element as HTMLInputElement).value).toBe('0.10');
    expect(w.get('#configure-limits').text()).toContain('limits.unpriced|{"provider":"Groq"}');
    await cost.setValue('abc');
    expect(w.get('#configure-limits').text()).toContain('limits.not_amount');
    await cost.setValue('');
    expect(w.get('#configure-limits').text()).not.toContain('limits.unpriced');
    await cost.setValue('0');
    expect(w.get('#configure-limits').text()).not.toContain('limits.unpriced');
    await cost.setValue('0.5');
    expect(w.get('#configure-limits').text()).toContain('limits.unpriced');
    expect(w.get('.mockup-code').text()).toContain('"maxCostCents":50');
    const { useKnowledgeStore } = await import('~/stores/knowledge');
    useKnowledgeStore().embeddingReady = false;
    await flushPromises();
    expect(w.get('#configure-memory').text()).toContain('configure.knowledge_needs_embedding');
  });

  it('guards a move to another agent\'s Configure page like a leave', async () => {
    const update = vi.fn();
    vi.stubGlobal('onBeforeRouteUpdate', update);
    inProject('/agents/support', [support], 'agent:read', 'agent:write');
    const client = withDraft();
    const w = await mountPage(ConfigurePage);
    expect(update).toHaveBeenCalledOnce();
    const guard = update.mock.calls[0]![0] as () => Promise<boolean>;
    client.updateAgentDraft.mockRejectedValue(new Error('offline'));
    await w.get('textarea').setValue('Help even more.');
    expect(await guard()).toBe(false);
    w.unmount();
  });

  it('keeps the API example in step with the real draft fields', async () => {
    inProject('/agents/support', [support], 'agent:read', 'agent:write');
    withDraft(0);
    const w = await mountPage(ConfigurePage);
    expect(w.get('[role="status"]').element.parentElement!.textContent).toContain('configure.never_published');
    expect(w.text()).not.toContain('legacy.banner');
    await button(w, 'publish.fix_count|{"n":2}').trigger('click');
    await flushPromises();
    expect(w.findAll('dialog').find(d => (d.element as HTMLDialogElement).open)!.text()).toContain('publish.blocker_model');
    const api = () => w.get('.mockup-code').text();
    expect(w.get('.mockup-code pre').attributes('data-prefix')).toBe('$');
    expect(api()).toContain('"$OAGENT_URL/api/agent.v1.AgentService/UpdateAgentDraft"');
    expect(api()).toContain('"mcpServers":[]');
    expect(api()).not.toContain('acme.orders.read');
    expect(w.get('[role="status"]').text()).not.toContain('configure.unsaved');
    await w.get('input[inputmode="decimal"]').setValue('0.5');
    expect(api()).toContain('"maxCostCents":50');
    expect(w.get('[role="status"]').text()).toContain('configure.unsaved');
    await w.get('textarea').setValue('Don\'t promise a date.');
    expect(api()).toContain('Don\'\\\'\'t promise a date.');
    expect(api()).not.toContain('Don\'t');
    expect(w.findAll('input.tab').map(i => i.attributes('aria-label'))).toEqual(['configure.api_tab', 'configure.mcp_tab']);
    await w.get('input[value="mcp"]').setValue(true);
    expect(w.findAll('.mockup-code')[1]!.text()).toContain('"tool": "agent_update_draft"');
    expect(w.findAll('.mockup-code')[1]!.find('pre').attributes('data-prefix')).toBe('1');
  });

  it('is read-only without agent:write', async () => {
    inProject('/agents/support', [support], 'agent:read');
    withDraft(12, { draft: { ...live, tools: [{ name: 'custom.tool', requireApproval: true }] } });
    const w = await mountPage(ConfigurePage);
    expect(w.findAll('button').some(b => b.text() === 'configure.publish')).toBe(false);
    expect(w.findAll('button').some(b => b.text() === 'legacy.pick_model')).toBe(false);
    expect(w.get('form fieldset').attributes('disabled')).toBeDefined();
    expect(w.text()).toContain('custom.tool');
  });

  it('opens a new agent with an empty config, never sample values', async () => {
    inProject('/agents/support', [support], 'agent:read', 'agent:write');
    withDraft(0, { draft: undefined, unpublishedChanges: false });
    const w = await mountPage(ConfigurePage);
    expect(w.get('textarea').element.value).toBe('');
    const [provider, model] = w.findAll('#configure-model select');
    expect((provider!.element as HTMLSelectElement).value).toBe('');
    expect(model!.attributes('disabled')).toBeDefined();
    expect(w.findAll('#configure-tools li input[type="checkbox"]:not(.toggle)').every(i => !(i.element as HTMLInputElement).checked)).toBe(true);
    expect(w.findAll('#configure-web input.toggle').map(i => (i.element as HTMLInputElement).checked)).toEqual([false, false]);
  });

  it('is a 404 for an unknown agent', async () => {
    inProject('/agents/support', [], 'agent:read');
    const errorHandler = vi.fn();
    await mountPage(ConfigurePage, { global: { config: { errorHandler } } });
    expect(errorHandler.mock.calls[0]![0]).toMatchObject({ statusCode: 404, statusMessage: 'error.agent_not_found' });
  });
});
