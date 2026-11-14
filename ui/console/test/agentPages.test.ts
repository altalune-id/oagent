// @vitest-environment happy-dom
import { create } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { flushPromises, RouterLinkStub } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { fakeKnowledgeClient } from '~~/test/fixtures/knowledge';
import { fakeRuntimeClients, providerPBs } from '~~/test/fixtures/runtime';
import { useToast } from '#base/components/ui/toast';
import { WhoamiResponseSchema } from '#gen/auth/v1/auth_pb';
import { ModelProviderKind, ModelProviderSchema } from '#gen/modelprovider/v1/modelprovider_pb';
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
  fakeKnowledgeClient();
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

// agentDefaults differ from the shipped ones, so a hard-coded number fails a test.
const agentDefaults = { maxSteps: 25, maxMaxSteps: 60, maxTemperature: 1.5, maxCostCents: 50_000, voiceNoteMaxSeconds: 240, voiceNoteMaxBytes: 3 * 1024 * 1024 };

function withDraft(publishedVersion = 12, extra: Record<string, unknown> = {}, liveConfig = live) {
  const agent = { ...support, draft: { ...live, instructions: 'Help more.' }, publishedVersion, unpublishedChanges: true, ...extra };
  const client = {
    listAgents: vi.fn().mockResolvedValue({ agents: [support] }),
    getAgent: vi.fn().mockResolvedValue({ agent, defaults: agentDefaults }),
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
    client.getAgent.mockResolvedValue({ agent: { ...yasaku, draft, publishedVersion: 4, unpublishedChanges: true }, defaults: agentDefaults });
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
    expect(nav.findAll('a').map(a => a.attributes('href'))).toEqual(['#configure-instructions', '#configure-model', '#configure-voice', '#configure-tools', '#configure-web', '#configure-memory', '#configure-limits']);
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

  it('disables the temperature for an Anthropic provider, keeps its value, and enables it again for OpenAI-compatible', async () => {
    inProject('/agents/support', [support], 'agent:read', 'agent:write');
    withDraft();
    const w = await mountPage(ConfigurePage, { attachTo: document.body });
    const range = () => w.get('#configure-model input[type="range"]');
    const hint = () => w.get(`#${range().attributes('aria-describedby')}`);
    const [provider] = w.findAll('#configure-model select');
    await provider!.setValue('Pv4cLd8NwQ2xTr6YbMa9F');
    expect(range().attributes('disabled')).toBeDefined();
    expect(range().attributes('aria-valuetext')).toBe('configure.temperature_off');
    expect(hint().text()).toBe('configure.temperature_unsupported');
    expect((range().element as HTMLInputElement).value).toBe('0.2');
    await provider!.setValue(openrouter);
    expect(range().attributes('disabled')).toBeUndefined();
    expect(range().attributes('aria-valuetext')).toBe('0.2');
    expect(hint().text()).toBe('configure.temperature_hint');
    w.unmount();
  });

  it('offers the main model as the empty helper choice, names it in the hint, and leaves it out of the helper list', async () => {
    asSample();
    const w = await mountPage(ConfigurePage);
    await flushPromises();
    const helper = w.get('#configure-model select[data-field="helper"]');
    const options = helper.findAll('option');
    expect(options[0]!.text()).toBe('configure.helper_same');
    expect(options[0]!.attributes('value')).toBe('');
    expect((helper.element as HTMLSelectElement).value).toBe('');
    expect(options.map(o => o.attributes('value'))).not.toContain('openai/gpt-4.1-mini');
    const hint = () => w.get(`#${helper.attributes('aria-describedby')!.split(' ')[0]}`).text();
    expect(hint()).toBe('configure.helper_uses|{"model":"openai/gpt-4.1-mini"} configure.helper_hint');
    expect(w.text()).not.toContain('configure.helper_none');
    await helper.setValue('google/gemini-2.5-flash');
    expect(hint()).toBe('configure.helper_hint');
    await helper.setValue('');
    await w.get('#configure-model select:not([data-field])').setValue('');
    expect(hint()).toBe('configure.helper_hint');
    w.unmount();
  });

  it('keeps a stored helper that equals the main model listed and selected, and never saves a change for it', async () => {
    vi.useFakeTimers();
    const client = asSample({ ...sampleDraft, helperModel: 'openai/gpt-4.1-mini' });
    const w = await mountPage(ConfigurePage);
    await flushPromises();
    const helper = w.get('#configure-model select[data-field="helper"]');
    expect((helper.element as HTMLSelectElement).value).toBe('openai/gpt-4.1-mini');
    expect(helper.findAll('option').map(o => o.attributes('value'))).toContain('openai/gpt-4.1-mini');
    expect(w.get(`#${helper.attributes('aria-describedby')!.split(' ')[0]}`).text()).toBe('configure.helper_hint');
    await vi.advanceTimersByTimeAsync(3000);
    expect(client.updateAgentDraft).not.toHaveBeenCalled();
    vi.useRealTimers();
    w.unmount();
  });

  it('puts an unpriced helper model\'s cap error on the helper field, and its fix focuses the helper', async () => {
    const { useModelProvidersStore } = await import('~/stores/modelProviders');
    asSample({ ...sampleDraft, helperModel: 'free-helper' });
    const w = await mountPage(ConfigurePage, { attachTo: document.body });
    useModelProvidersStore().items[0]!.models.push({ id: 'free-helper', vision: false, embedding: false });
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

  it('marks each tool that runs without approval once a knowledge source is bound, and warns at Publish without blocking it', async () => {
    asSample();
    const w = await mountPage(ConfigurePage, { attachTo: document.body });
    await flushPromises();
    const tools = w.get('#configure-tools');
    expect(tools.text()).not.toContain('grants.runs_unattended');
    await w.get('#configure-memory').findAll('li input')[0]!.setValue(true);
    expect(tools.text().match(/grants\.runs_unattended/g)).toHaveLength(3);
    await button(w, 'configure.publish').trigger('click');
    const dialog = openDialog();
    const warning = dialog.querySelector('[role="status"] li button')!;
    expect(warning.textContent!.trim()).toBe('publish.warning_unapproved_tool|{"tool":"yasaku__list_wallets","n":"2"}');
    expect(dialog.querySelector('[role="alert"]')).toBeNull();
    expect([...dialog.querySelectorAll('button')].find(b => b.textContent?.includes('configure.publish_n'))!.disabled).toBe(false);
    w.unmount();
  });

  it('jumps from the warning to the tool, opening its server', async () => {
    asSample({ ...sampleDraft, knowledgeSourceIds: ['Ks1Upl0adQ8sWx3YcLm7D'] });
    const w = await mountPage(ConfigurePage, { attachTo: document.body });
    await flushPromises();
    const yasakuTools = w.get('#configure-tools').findAll('details')[0]!.element as HTMLDetailsElement;
    expect(yasakuTools.open).toBe(false);
    await button(w, 'configure.publish').trigger('click');
    (openDialog().querySelector('[role="status"] li button') as HTMLButtonElement).click();
    await flushPromises();
    expect(yasakuTools.open).toBe(true);
    expect(document.activeElement).toBe(yasakuTools.querySelector('[data-field="yasaku__list_wallets"]'));
    w.unmount();
  });

  it('places an unapproved_tool refusal under the named tool', async () => {
    setKnownKeys('apperror.AGT007_unapproved_tool');
    const client = asSample({ ...sampleDraft, knowledgeSourceIds: ['Ks1Upl0adQ8sWx3YcLm7D'] });
    client.publishAgent.mockRejectedValue(connectError('AGT007', { reason: 'unapproved_tool', name: 'yasaku__search_tx' }));
    const w = await mountPage(ConfigurePage, { attachTo: document.body });
    await flushPromises();
    await button(w, 'configure.publish').trigger('click');
    [...openDialog().querySelectorAll('button')].find(b => b.textContent?.includes('configure.publish_n'))!.click();
    await flushPromises();
    const message = 'apperror.AGT007_unapproved_tool|{"name":"yasaku__search_tx"}';
    expect(openDialog().textContent).toContain(message);
    const tools = w.get('#configure-tools');
    expect(tools.text().match(/apperror\.AGT007_unapproved_tool/g)).toHaveLength(1);
    const box = tools.get('[data-field="yasaku__search_tx"]');
    const errId = box.attributes('aria-describedby')!.split(' ').find(id => document.getElementById(id)?.textContent?.trim() === message);
    expect(errId).toBeDefined();
    expect(tools.get('[data-field="yasaku__list_wallets"]').attributes('aria-describedby') ?? '').not.toContain(errId!);
    expect(box.attributes('aria-invalid')).toBe('true');
    expect(tools.get('[data-field="yasaku__list_wallets"]').attributes('aria-invalid')).toBeUndefined();
    w.unmount();
  });

  it('shows an unapproved_tool refusal once at the top of the tools when its row is not listed', async () => {
    setKnownKeys('apperror.AGT007_unapproved_tool');
    const client = asSample({ ...sampleDraft, knowledgeSourceIds: ['Ks1Upl0adQ8sWx3YcLm7D'] });
    client.publishAgent.mockRejectedValue(connectError('AGT007', { reason: 'unapproved_tool', name: 'tagihan__list_bills' }));
    const w = await mountPage(ConfigurePage, { attachTo: document.body });
    await flushPromises();
    await button(w, 'configure.publish').trigger('click');
    [...openDialog().querySelectorAll('button')].find(b => b.textContent?.includes('configure.publish_n'))!.click();
    await flushPromises();
    expect(w.get('#configure-tools').text().match(/apperror\.AGT007_unapproved_tool/g)).toHaveLength(1);
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

  it('picks memory with a store select that is off by default and links to the stores page', async () => {
    asSample();
    const w = await mountPage(ConfigurePage);
    const memory = w.get('#configure-memory');
    const select = memory.get('select[data-field="memory"]');
    expect((select.element as HTMLSelectElement).value).toBe('');
    expect(select.findAll('option')[0]!.text()).toBe('configure.memory_off');
    expect(memory.text()).toContain('configure.memory_hint');
    const manage = memory.findAllComponents(RouterLinkStub).find(l => l.props('to') === '/o/acme/projects/web/memory-stores')!;
    expect(manage.classes()).toContain('min-h-6');
    expect(memory.text()).not.toContain('configure.memory_see');
    w.unmount();
  });

  it('saves a picked memory store with the draft and lists it at Publish', async () => {
    vi.useFakeTimers();
    const client = asSample();
    const w = await mountPage(ConfigurePage, { attachTo: document.body });
    const select = w.get('#configure-memory select[data-field="memory"]');
    expect(select.findAll('option').map(o => o.text())).toEqual(['configure.memory_off', 'People']);
    await select.setValue('Ms4tR7wQ2xKp9VbNc3HzL');
    await vi.advanceTimersByTimeAsync(1000);
    expect(client.updateAgentDraft).toHaveBeenLastCalledWith(expect.objectContaining({ config: expect.objectContaining({ memoryStoreId: 'Ms4tR7wQ2xKp9VbNc3HzL' }) }));
    expect(w.get('#configure-memory').text()).toContain('configure.memory_see');
    await button(w, 'configure.publish').trigger('click');
    expect(openDialog().textContent).toContain('configure.change_memory_on');
    vi.useRealTimers();
    w.unmount();
  });

  it('says when the bound memory store was deleted, and shows a memory_store refusal under the select', async () => {
    vi.useFakeTimers();
    setKnownKeys('apperror.AGT007_memory_store_disabled');
    const client = asSample({ ...sampleDraft, memoryStoreMissing: true });
    client.updateAgentDraft.mockRejectedValue(connectError('AGT007', { reason: 'memory_store_disabled' }));
    const w = await mountPage(ConfigurePage);
    const memory = w.get('#configure-memory');
    expect(memory.text()).toContain('configure.memory_store_missing');
    await w.get('#configure-instructions textarea').setValue('Catat semua.');
    await vi.advanceTimersByTimeAsync(1000);
    await flushPromises();
    const select = memory.get('select[data-field="memory"]');
    expect(select.attributes('aria-invalid')).toBe('true');
    const err = select.attributes('aria-describedby')!.split(' ').map(id => w.find(`#${id}`)).find(e => e.exists() && e.text() === 'apperror.AGT007_memory_store_disabled');
    expect(err).toBeDefined();
    expect(w.text().match(/apperror\.AGT007_memory_store_disabled/g)).toHaveLength(1);
    vi.useRealTimers();
  });

  describe('voice notes', () => {
    const sttId = 'Pv2sTt7QxR4mLk8WnB3cZ';
    const zero = { inputMicros: 0n, outputMicros: 0n, cacheReadMicros: 0n, cacheWriteMicros: 0n };
    function withStt() {
      runtimeClients.provider.listModelProviders.mockResolvedValue({ modelProviders: [...providerPBs(), create(ModelProviderSchema, { id: sttId, orgId, projectId, name: 'Speech', kind: ModelProviderKind.OPENAI_COMPATIBLE, baseUrl: 'https://api.openai.com/v1', apiKeySet: true, models: [{ id: 'whisper-1', transcription: true, prices: { ...zero, audioMinuteMicros: 6_000n } }, { id: 'free-stt', transcription: true, prices: zero }] })] });
    }
    const voiceSelect = (w: Awaited<ReturnType<typeof mountPage>>) => w.get('#configure-voice select[data-field="voice"]');

    it('lists Voice notes right after Model in the section nav', async () => {
      asSample();
      const w = await mountPage(ConfigurePage);
      const labels = w.findAll('nav li a').map(a => a.text());
      expect(labels.indexOf('configure.voice')).toBe(labels.indexOf('configure.model') + 1);
      w.unmount();
    });

    it('keeps only Off, disabled, with a providers link when no model is marked Transcription', async () => {
      asSample();
      const w = await mountPage(ConfigurePage);
      const select = voiceSelect(w);
      expect(select.findAll('option').map(o => o.text())).toEqual(['configure.voice_off']);
      expect(select.attributes('disabled')).toBeDefined();
      const voice = w.get('#configure-voice');
      expect(voice.text()).toContain('configure.voice_no_models');
      expect(voice.findAllComponents(RouterLinkStub).some(l => l.props('to') === '/o/acme/projects/web/settings/model-providers')).toBe(true);
      const hint = select.attributes('aria-describedby')!.split(' ')[0]!;
      expect(w.get(`#${hint}`).element.textContent!.trim().split(/\s+(?=configure\.|providers\.)/)).toEqual(['configure.voice_no_models', 'configure.voice_hint|{"max_minutes":"4","max_size":"3 MiB","reply":"channels.reply_voice"}', 'configure.voice_reply_link|{"reply":"channels.reply_voice"}', 'providers.title']);
      w.unmount();
    });

    it('offers each transcription model by provider, saves the pick with its price, limits and links, and lists it at Publish', async () => {
      vi.useFakeTimers();
      withStt();
      const client = asSample();
      const w = await mountPage(ConfigurePage, { attachTo: document.body });
      const select = voiceSelect(w);
      expect(select.findAll('option').map(o => o.text())).toEqual(['configure.voice_off', 'configure.voice_option|{"model":"free-stt","provider":"Speech"}', 'configure.voice_option|{"model":"whisper-1","provider":"Speech"}']);
      const voice = w.get('#configure-voice');
      expect(voice.text()).toContain('configure.voice_hint|{"max_minutes":"4","max_size":"3 MiB","reply":"channels.reply_voice"}');
      const hrefs = voice.findAllComponents(RouterLinkStub).map(l => l.props('to'));
      expect(hrefs).toEqual(expect.arrayContaining(['/o/acme/projects/web/agents/yasaku-keuangan/channels', '/o/acme/projects/web/settings/model-providers']));
      await select.setValue(`${sttId}|whisper-1`);
      expect(voice.text()).toContain('configure.voice_price|{"price":"0.006"}');
      const hint = w.get(`#${select.attributes('aria-describedby')!.split(' ')[0]}`).element.textContent!.trim();
      expect(hint.split(/\s+(?=configure\.|providers\.)/).map(s => s.split('|')[0])).toEqual(['configure.voice_price', 'configure.voice_hint', 'configure.voice_reply_link', 'providers.title']);
      await vi.advanceTimersByTimeAsync(1000);
      expect(client.updateAgentDraft).toHaveBeenLastCalledWith(expect.objectContaining({ config: expect.objectContaining({ transcriptionModel: { providerId: sttId, model: 'whisper-1' } }) }));
      await select.setValue(`${sttId}|free-stt`);
      expect(voice.text()).toContain('configure.voice_free');
      await button(w, 'configure.publish').trigger('click');
      expect(openDialog().textContent).toContain('configure.change_transcription_on|{"model":"free-stt · Speech"}');
      vi.useRealTimers();
      w.unmount();
    });

    it('turns voice notes off by picking Off', async () => {
      vi.useFakeTimers();
      withStt();
      const client = asSample({ ...sampleDraft, transcriptionModel: { providerId: sttId, model: 'whisper-1', providerMissing: false } });
      const w = await mountPage(ConfigurePage);
      expect((voiceSelect(w).element as HTMLSelectElement).value).toBe(`${sttId}|whisper-1`);
      await voiceSelect(w).setValue('');
      await vi.advanceTimersByTimeAsync(1000);
      expect(client.updateAgentDraft.mock.calls.at(-1)![0].config).not.toHaveProperty('transcriptionModel');
      vi.useRealTimers();
      w.unmount();
    });

    it('keeps a model whose provider was deleted as a stale option and says it turns off at the next save', async () => {
      withStt();
      asSample({ ...sampleDraft, transcriptionModel: { providerId: '', model: 'whisper-1', providerMissing: true } });
      const w = await mountPage(ConfigurePage);
      const select = voiceSelect(w);
      expect(select.findAll('option').map(o => o.text())).toContain('whisper-1');
      expect((select.element as HTMLSelectElement).selectedOptions[0]!.text).toBe('whisper-1');
      expect(w.get('#configure-voice').text()).toContain('configure.voice_provider_missing|{"model":"whisper-1"}');
      w.unmount();
    });

    it('warns when the set model is no longer marked Transcription, keeping it as a stale option', async () => {
      withStt();
      asSample({ ...sampleDraft, transcriptionModel: { providerId: openrouter, model: 'openai/gpt-4.1-mini', providerMissing: false } });
      const w = await mountPage(ConfigurePage);
      expect((voiceSelect(w).element as HTMLSelectElement).selectedOptions[0]!.text).toBe('configure.voice_option|{"model":"openai/gpt-4.1-mini","provider":"OpenRouter"}');
      expect(w.get('#configure-voice').text()).toContain('configure.voice_model_removed|{"model":"openai/gpt-4.1-mini","provider":"OpenRouter"}');
      w.unmount();
    });

    it('places an AGT007 transcription_model refusal under the voice notes select', async () => {
      vi.useFakeTimers();
      withStt();
      setKnownKeys('apperror.AGT007_transcription_model');
      const client = asSample();
      client.updateAgentDraft.mockRejectedValue(connectError('AGT007', { reason: 'transcription_model' }));
      const w = await mountPage(ConfigurePage);
      await voiceSelect(w).setValue(`${sttId}|whisper-1`);
      await vi.advanceTimersByTimeAsync(1000);
      await flushPromises();
      const select = voiceSelect(w);
      expect(select.attributes('aria-invalid')).toBe('true');
      expect(select.classes()).toContain('select-error');
      const err = select.attributes('aria-describedby')!.split(' ').map(id => w.find(`#${id}`)).find(e => e.exists() && e.text() === 'apperror.AGT007_transcription_model');
      expect(err).toBeDefined();
      expect(w.text().match(/apperror\.AGT007_transcription_model/g)).toHaveLength(1);
      vi.useRealTimers();
    });
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
    const always = untrusted.get('li input[aria-label="approvalrule.always"]');
    expect((always.element as HTMLInputElement).checked, 'a read-only hint on an untrusted server still asks').toBe(true);
    expect(untrusted.find('[role="alert"]').exists()).toBe(false);
    await untrusted.get('li input[aria-label="approvalrule.never"]').setValue(true);
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
    expect(sources.map(i => [(i.element as HTMLInputElement).checked, i.attributes('disabled') !== undefined])).toEqual([[false, false], [false, true], [false, true]]);
    expect(memory.text()).toContain('configure.knowledge_ready_docs|{"n":2}');
    expect(memory.text()).toContain('knowledge.source_crawling');
    await sources[0]!.setValue(true);
    expect(w.get('.mockup-code').text()).toContain('"knowledgeSourceIds":["Ks1Upl0adQ8sWx3YcLm7D"]');
    const { useMcpServersStore } = await import('~/stores/mcpServers');
    useMcpServersStore().items = useMcpServersStore().items.filter(x => x.name !== 'yasaku');
    await flushPromises();
    expect(w.get('#configure-tools').text()).toContain('grants.server_gone|{"n":7}');
    await button(w, 'configure.view_changes').trigger('click');
    expect(openDialog().textContent).toContain('publish.blocker_server_missing');
    expect(openDialog().textContent).toContain('configure.change_knowledge');
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

  it('lets a bound source that is no longer ready be unbound, and keeps it bound otherwise', async () => {
    asSample({ ...sampleDraft, knowledgeSourceIds: ['Ks2Ur1CrawlTq9Lm4Xw8P'] });
    const w = await mountPage(ConfigurePage);
    const crawling = w.get('#configure-memory').findAll('li input')[1]!;
    expect([(crawling.element as HTMLInputElement).checked, crawling.attributes('disabled')]).toEqual([true, undefined]);
    await crawling.setValue(false);
    expect(w.get('.mockup-code').text()).toContain('"knowledgeSourceIds":[]');
    expect(crawling.attributes('disabled')).toBeDefined();
    expect(w.get('#configure-memory').text()).not.toContain('configure.knowledge_missing');
  });

  it('offers no embedding-only model as the chat or helper model, and explains knowledge with a link', async () => {
    asSample();
    const { useModelProvidersStore } = await import('~/stores/modelProviders');
    const w = await mountPage(ConfigurePage);
    useModelProvidersStore().items[0]!.models.push({ id: 'text-embedding-3-small', vision: false, embedding: true });
    await flushPromises();
    for (const select of w.findAll('#configure-model select').slice(1)) {
      expect(select.findAll('option').map(o => o.attributes('value'))).not.toContain('text-embedding-3-small');
    }
    const memory = w.get('#configure-memory');
    expect(memory.text()).toContain('configure.knowledge_hint');
    expect(memory.findAllComponents(RouterLinkStub).map(l => l.props('to'))).toContain('/o/acme/projects/web/knowledge');
  });

  it('tunes knowledge search with server presets and recommended values, unset by default', async () => {
    setKnownKeys('configure.preset_precise', 'configure.preset_balanced', 'configure.preset_broad');
    asSample({ ...sampleDraft, knowledgeSourceIds: ['Ks1Upl0adQ8sWx3YcLm7D'] });
    const w = await mountPage(ConfigurePage);
    const tuning = w.get('[data-tuning]');
    const chips = tuning.findAll('input[type="radio"]');
    expect(chips.map(c => [c.element.closest('label')!.textContent!.trim(), (c.element as HTMLInputElement).checked])).toEqual([
      ['configure.preset_precise', false],
      ['configure.preset_balanced', true],
      ['configure.preset_broad', false],
    ]);
    expect(tuning.text()).toContain('configure.tuning_recommended|{"n":"6"}');
    expect(tuning.text()).toContain('configure.tuning_recommended|{"n":"0.25"}');
    expect(tuning.text()).toContain('configure.tuning_recommended|{"n":"3,000"}');
    await chips[0]!.setValue(true);
    expect(w.get('.mockup-code').text()).toContain('"knowledgeTuning":{"topK":3,"maxTokens":0,"minScore":0.35}');
    const [topK] = tuning.findAll('input[type="number"]');
    await topK!.setValue('7');
    expect(tuning.text()).toContain('configure.preset_custom');
    expect(chips.every(c => !(c.element as HTMLInputElement).checked)).toBe(true);
    await topK!.setValue('11');
    expect(tuning.text()).toContain('configure.tuning_range|{"min":1,"max":10}');
    expect(w.get('.mockup-code').text(), 'an out-of-range value never reaches the draft').toContain('"topK":7');
    await tuning.findAll('button').find(b => b.text() === 'configure.tuning_reset')!.trigger('click');
    expect(w.get('.mockup-code').text()).toContain('"knowledgeTuning":{"topK":0,"maxTokens":0}');
    expect((chips[1]!.element as HTMLInputElement).checked).toBe(true);
    await w.findAll('button').find(b => b.text() === 'configure.view_changes')!.trigger('click');
    expect([...document.querySelectorAll('dialog')].find(d => d.open)?.textContent ?? '').not.toContain('configure.change_knowledge_tuning');
  });

  it('sends a typed minimum relevance of 0, and keeps the saved tuning when the last source is unbound', async () => {
    asSample({ ...sampleDraft, knowledgeSourceIds: ['Ks1Upl0adQ8sWx3YcLm7D'], knowledgeTuning: { topK: 3, maxTokens: 0 } });
    const w = await mountPage(ConfigurePage);
    const tuning = w.get('[data-tuning]');
    await tuning.findAll('input[type="number"]')[1]!.setValue('0');
    expect(w.get('.mockup-code').text()).toContain('"knowledgeTuning":{"topK":3,"maxTokens":0,"minScore":0}');
    await w.get('#configure-memory li input[type="checkbox"]').setValue(false);
    expect(w.get('.mockup-code').text()).toContain('"knowledgeSourceIds":[],"knowledgeTuning":{"topK":3,"maxTokens":0,"minScore":0}');
    expect(tuning.text()).toContain('configure.tuning_needs_source');
    expect(tuning.findAll('input').every(i => i.attributes('disabled') !== undefined)).toBe(true);
  });

  it('turns tuning off with a hint while no source is bound', async () => {
    asSample();
    const w = await mountPage(ConfigurePage);
    const tuning = w.get('[data-tuning]');
    expect(tuning.text()).toContain('configure.tuning_needs_source');
    expect(tuning.findAll('input').every(i => i.attributes('disabled') !== undefined)).toBe(true);
    expect(tuning.findAll('button').every(b => b.attributes('disabled') !== undefined)).toBe(true);
  });

  it('says how many bound knowledge sources were deleted', async () => {
    asSample({ ...sampleDraft, knowledgeSourcesMissing: 2 });
    const w = await mountPage(ConfigurePage);
    expect(w.get('#configure-memory [role="note"]').text()).toBe('configure.knowledge_missing|{"n":2}');
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
    useKnowledgeStore().embedding = null;
    await flushPromises();
    expect(w.get('#configure-memory').text()).toContain('configure.knowledge_needs_embedding');
  });

  it('bounds the steps, temperature and cost cap by the server\'s defaults, and names the recommended steps', async () => {
    asSample();
    const w = await mountPage(ConfigurePage);
    const steps = w.get('#configure-limits input[type="number"]');
    expect(steps.attributes('max')).toBe('60');
    expect(w.get(`#${steps.attributes('aria-describedby')}`).text()).toBe('configure.max_steps_hint|{"n":25}');
    expect(w.get('#configure-model input[type="range"]').attributes('max')).toBe('1.5');
    const cost = w.get('#configure-limits input[inputmode="decimal"]');
    await cost.setValue('500');
    expect(w.get('#configure-limits').text()).not.toContain('limits.max_cost_over');
    await cost.setValue('500.01');
    expect(cost.attributes('aria-invalid')).toBe('true');
    expect(w.get('#configure-limits').text()).toContain('limits.max_cost_over|{"max":"$500.00"}');
    expect(w.get('.mockup-code').text(), 'an amount over the cap is not saved').toContain('"maxCostCents":50000');
    expect(button(w, 'configure.publish').attributes('disabled'), 'an amount over the cap blocks Publish').toBeDefined();
    expect(button(w, 'publish.fix_count|{"n":1}').exists()).toBe(true);
    await cost.setValue('400');
    expect(button(w, 'configure.publish').attributes('disabled')).toBeUndefined();
  });

  for (const [label, defaults] of [['missing', undefined], ['all 0', { maxSteps: 0, maxMaxSteps: 0, maxTemperature: 0, maxCostCents: 0 }]] as const) {
    it(`sets no bound, hint or cap check from ${label} agent defaults, and keeps the temperature off`, async () => {
      const client = asSample();
      client.getAgent.mockResolvedValue({ agent: { ...yasaku, draft: sampleDraft, publishedVersion: 4, unpublishedChanges: true }, defaults });
      const w = await mountPage(ConfigurePage);
      const steps = w.get('#configure-limits input[type="number"]');
      expect(steps.attributes('max')).toBeUndefined();
      expect(steps.attributes('aria-describedby')).toBeUndefined();
      expect(w.get('#configure-limits').text()).not.toContain('configure.max_steps_hint');
      const range = w.get('#configure-model input[type="range"]');
      expect(range.attributes('max')).toBeUndefined();
      expect(range.attributes('disabled')).toBeDefined();
      await w.get('#configure-limits input[inputmode="decimal"]').setValue('999999');
      expect(w.get('#configure-limits').text()).not.toContain('limits.max_cost_over');
    });
  }

  it('keeps the temperature off and sets no bounds after a failed load', async () => {
    const client = asSample();
    client.getAgent.mockRejectedValue(new Error('offline'));
    const w = await mountPage(ConfigurePage);
    expect(w.get('#configure-model input[type="range"]').attributes('disabled')).toBeDefined();
    expect(w.get('#configure-model input[type="range"]').attributes('max')).toBeUndefined();
    expect(w.get('#configure-limits input[type="number"]').attributes('max')).toBeUndefined();
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
