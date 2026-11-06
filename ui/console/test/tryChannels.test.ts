// @vitest-environment happy-dom
import { create } from '@bufbuild/protobuf';
import { flushPromises } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { fakeRuntimeClients } from '~~/test/fixtures/runtime';
import { WhoamiResponseSchema } from '#gen/auth/v1/auth_pb';
import { mountPage } from '#test/mount-page';
import { setClient, setRoute } from '#test/nuxt-globals';
import ChannelsPage from '~/pages/o/[org]/projects/[project]/agents/[agent]/channels.vue';
import TryPage from '~/pages/o/[org]/projects/[project]/agents/[agent]/try.vue';
import { useChannelsStore } from '~/stores/channels';
import { useModelProvidersStore } from '~/stores/modelProviders';
import { useOrgsStore } from '~/stores/orgs';
import { useProjectsStore } from '~/stores/projects';

const orgId = 'B6rTn1YkM8wQz3XpL5vGd';
const projectId = 'x6MH9WPWjktMwCBKHy4pF';
const yasaku = { id: 'Y4sKu8RmT2pLq6WnX9cVb', projectId, slug: 'yasaku-keuangan', name: 'Yasaku Keuangan', description: '', publishedVersion: 4 };
const draftConfig = { instructions: 'Halo', providerId: 'Pv6tQm2ZxR8wKp3VbNc4K', model: 'openai/gpt-4.1-mini', helperModel: '', temperature: 0.3, maxSteps: 12, maxCostCents: 20, mcpServers: [], providerMissing: false, tools: [] as { name: string, requireApproval: boolean }[] };

function seed(agent: Record<string, unknown> = yasaku, permissions = ['agent:read', 'agent:write'], tools = draftConfig.tools) {
  useOrgsStore().items = [{ id: orgId, slug: 'acme', name: 'Acme', role: 'owner', permissions: [] }];
  useProjectsStore().items = [{ id: projectId, orgId, slug: 'rumah', name: 'Rumah Tangga' }];
  setRoute({ path: `/o/acme/projects/rumah/agents/${agent.slug}/try`, params: { org: 'acme', project: 'rumah', agent: agent.slug as string } });
  setClient('$authClient', { whoami: vi.fn().mockResolvedValue(create(WhoamiResponseSchema, { name: 'Budi', permissions })) });
  setClient('$agentClient', {
    listAgents: vi.fn().mockResolvedValue({ agents: [agent] }),
    getAgent: vi.fn().mockResolvedValue({ agent: { ...agent, draft: { ...draftConfig, tools }, unpublishedChanges: true } }),
    listAgentVersions: vi.fn().mockResolvedValue({ versions: [] }),
  });
}

const button = (w: Awaited<ReturnType<typeof mountPage>>, text: string) => w.findAll('button').find(b => b.text().includes(text))!;

beforeEach(() => {
  setActivePinia(createPinia());
  fakeRuntimeClients();
  vi.stubGlobal('definePageMeta', vi.fn());
});

afterEach(() => {
  vi.useRealTimers();
});

describe('try it page', () => {
  it('starts empty with suggestions, a disabled live tab and the run pane', async () => {
    seed();
    const w = await mountPage(TryPage);
    expect(w.get('h1').text()).toBe('try.title');
    expect(w.text()).toContain('try.empty');
    const tabs = w.findAll('label.tab');
    expect(tabs.map(l => l.text())).toEqual(['try.draft', 'try.live_legacy|{"n":4}']);
    expect(tabs[1]!.get('input').attributes('disabled')).toBeDefined();
    expect(tabs[1]!.classes()).toContain('tab-disabled');
    expect(w.text()).toContain('try.live_legacy_hint|{"n":4}');
    expect(w.findAll('li button').map(b => b.text())).toContain('beli kopi kenangan 40rb pakai cash');
    expect(w.get('aside').text()).toContain('try.phase_idle');
    expect(w.text()).not.toContain('try.no_vision');
  });

  it('plays the sample chat from a suggestion through the approval card', async () => {
    vi.useFakeTimers();
    seed();
    const w = await mountPage(TryPage);
    await w.findAll('li button')[0]!.trigger('click');
    await vi.runAllTimersAsync();
    await flushPromises();
    expect(w.get('[role="log"]').text()).toContain('beli kopi kenangan 40rb pakai cash');
    const card = w.get('[role="log"] section');
    expect(card.get('h3').text()).toBe('Simpan Rp40.000 dari Cash?');
    expect(card.findAll('dt').map(d => d.text())).toEqual(['Jumlah', 'Kategori', 'Dompet', 'Tanggal']);
    const approve = card.findAll('button').find(b => b.text().includes('approvalcard.approve_run'))!;
    expect(approve.text()).toContain('record_expense');
    expect(approve.attributes('aria-describedby')).toBe(card.get('dl').attributes('id'));
    expect(w.get('aside').text()).toContain('runstatus.awaiting_approval');
    expect(w.findAll('aside tbody tr').map(r => r.findAll('td')[1]!.text())).toEqual(['try.tool_done', 'try.tool_preview', 'try.tool_awaiting']);
    await approve.trigger('click');
    await vi.runAllTimersAsync();
    await flushPromises();
    expect(w.get('[role="log"]').text()).toContain('Tersimpan: Kopi Kenangan Rp40.000 dari Cash.');
    expect(card.text()).toContain('approvalcard.approved|{"by":"Budi"} · approvalcard.via_try');
    expect(w.get('aside').text()).toContain('runstatus.completed');
    await button(w, 'try.new_chat').trigger('click');
    expect(w.text()).toContain('try.empty');
  });

  it('rejects with a reason, sends with Enter and keeps Shift+Enter for a new line', async () => {
    vi.useFakeTimers();
    seed();
    const w = await mountPage(TryPage);
    const box = w.get('textarea');
    await box.setValue('Transfer 500rb dari BCA ke Cash');
    await box.trigger('keydown', { key: 'Enter', shiftKey: true });
    expect(w.text()).toContain('try.empty');
    await box.trigger('keydown', { key: 'Enter' });
    await vi.runAllTimersAsync();
    await flushPromises();
    const card = w.get('[role="log"] section');
    await button(w, 'approvalcard.reject').trigger('click');
    await card.get('input').setValue('salah');
    await card.findAll('button').find(b => b.text() === 'approvalcard.reject_send')!.trigger('click');
    await vi.runAllTimersAsync();
    await flushPromises();
    expect(card.text()).toContain('“salah”');
    expect(w.get('[role="log"]').text()).toContain('Oke, transfernya tidak saya catat.');
  });

  it('shows Stop while replying, then Retry after a model failure', async () => {
    vi.useFakeTimers();
    seed();
    const w = await mountPage(TryPage);
    await w.get('aside select').setValue('model_fails');
    await w.get('textarea').setValue('Pengeluaran makan bulan ini berapa?');
    await w.get('form').trigger('submit');
    expect(button(w, 'try.stop').exists()).toBe(true);
    expect(w.get('textarea').attributes('placeholder')).toBe('try.replying');
    await vi.runAllTimersAsync();
    await flushPromises();
    expect(w.text()).toContain('try.model_failed');
    await button(w, 'common.retry').trigger('click');
    await vi.runAllTimersAsync();
    await flushPromises();
    expect(w.get('[role="log"]').text()).toContain('Rp1.240.000');
  });

  it('refuses a big photo and notes when the model cannot read photos', async () => {
    seed();
    const w = await mountPage(TryPage);
    const input = w.get('input[type="file"]');
    Object.defineProperty(input.element, 'files', { value: [{ name: 'big.jpg', size: 6 * 1024 * 1024, type: 'image/jpeg' }] });
    await input.trigger('change');
    expect(w.text()).toContain('try.photo_too_big');
    useModelProvidersStore().items[0]!.models[0]!.vision = false;
    await flushPromises();
    expect(w.text()).toContain('try.no_vision');
    expect(w.get('button[aria-label="try.attach"]').attributes('disabled')).toBeDefined();
  });

  it('says why the draft cannot run instead of chatting', async () => {
    seed(yasaku, ['agent:read', 'agent:write'], [{ name: 'yasaku.record_expense', requireApproval: true }]);
    const w = await mountPage(TryPage);
    expect(w.get('[role="alert"]').text()).toContain('try.cant_run');
    expect(w.get('[role="alert"]').text()).toContain('publish.blocker_legacy_tools');
    expect(w.find('textarea').exists()).toBe(false);
  });

  it('is a 404 for an unknown agent', async () => {
    seed();
    setRoute({ path: '/o/acme/projects/rumah/agents/nope/try', params: { org: 'acme', project: 'rumah', agent: 'nope' } });
    const errorHandler = vi.fn();
    await mountPage(TryPage, { global: { config: { errorHandler } } });
    expect(errorHandler.mock.calls[0]![0]).toMatchObject({ statusCode: 404 });
  });
});

describe('channels page', () => {
  it('connects WhatsApp in three steps, then shows the connection and disconnects', async () => {
    vi.useFakeTimers();
    seed();
    const w = await mountPage(ChannelsPage, { attachTo: document.body });
    expect(w.findAll('section h2').map(h => h.text())).toEqual(['openwa.title', 'channels.provider_whatsapp_business', 'apiaccess.title', 'widget.title']);
    expect(w.text()).toContain('openwa.not_connected');
    expect(w.text()).toContain('openwa.status_none');
    expect(w.text()).not.toContain('openwa.publish_warning');
    expect(w.text()).toContain('widget.coming_later');
    expect(w.get('.mockup-code').text()).toContain(`/api/v1/agents/${yasaku.id}/runs`);
    await button(w, 'openwa.connect').trigger('click');
    const dialog = () => w.findAll('dialog').find(d => (d.element as HTMLDialogElement).open)!;
    expect(dialog().text()).toContain('openwa.before_title');
    expect(dialog().text()).not.toContain('openwa.publish_warning');
    expect(dialog().findAll('button').some(b => b.text() === 'common.cancel')).toBe(true);
    expect(dialog().get('li.step[aria-current="step"]').text()).toBe('openwa.step_webhook');
    await dialog().findAll('button').find(b => b.text() === 'openwa.next')!.trigger('click');
    const inputs = dialog().findAll('form input');
    await inputs[0]!.setValue('https://openwa.example.id');
    await inputs[1]!.setValue('dev_yasaku-rumah');
    await inputs[2]!.setValue('whsec_1');
    await inputs[3]!.setValue('owk_1');
    await dialog().findAll('button').find(b => b.text() === 'openwa.next')!.trigger('click');
    await dialog().findAll('button').find(b => b.text().includes('openwa.save_test'))!.trigger('click');
    await vi.runAllTimersAsync();
    await flushPromises();
    expect(dialog().text()).toContain('openwa.result_connected');
    await dialog().findAll('button').find(b => b.text() === 'common.done')!.trigger('click');
    expect(w.text()).toContain('openwa.status_connected');
    expect(w.findAll('section h2').map(h => h.text())).not.toContain('channels.connections');
    expect(w.text()).not.toContain('openwa.publish_warning');
    expect(w.text()).toContain('dev_yasaku-rumah');
    await button(w, 'openwa.disconnect').trigger('click');
    await flushPromises();
    await w.findAll('dialog').find(d => d.text().includes('openwa.disconnect_body'))!.findAll('button').find(b => b.text() === 'openwa.disconnect')!.trigger('click');
    await flushPromises();
    expect(useChannelsStore().connection(yasaku.id)).toBeUndefined();
    w.unmount();
  });

  it('shows a rejected-messages problem with Replace secret, and Continue setup while waiting', async () => {
    seed();
    useChannelsStore().save(yasaku.id, { id: 'chn_1', provider: 'openwa', status: 'bad_signature', baseUrl: 'https://openwa.example.id', deviceId: 'dev_a', webhookUrl: 'https://x/hooks', rejected24h: 3, rejectedLastHour: 3 });
    const w = await mountPage(ChannelsPage);
    const problem = w.findAll('[role="alert"]').find(a => a.text().includes('openwa.problem_bad_signature|{"n":3}'))!;
    await problem.get('button').trigger('click');
    const dialog = w.findAll('dialog').find(d => (d.element as HTMLDialogElement).open)!;
    expect(dialog.text()).toContain('openwa.keep_old');
    await dialog.get('input[type="password"]').setValue('whsec_2');
    await dialog.findAll('button').find(b => b.text() === 'common.save')!.trigger('click');
    expect(useChannelsStore().connection(yasaku.id)!.status).toBe('connected');
    useChannelsStore().save(yasaku.id, { ...useChannelsStore().connection(yasaku.id)!, status: 'waiting' });
    await flushPromises();
    expect(w.text()).toContain('openwa.status_waiting');
    await button(w, 'openwa.continue_setup').trigger('click');
    expect(w.findAll('dialog').find(d => (d.element as HTMLDialogElement).open)!.get('li.step[aria-current="step"]').text()).toBe('openwa.step_check');
  });

  it('warns that messages get no reply while the agent has unpublished changes', async () => {
    seed({ ...yasaku, unpublishedChanges: true });
    const w = await mountPage(ChannelsPage);
    await button(w, 'openwa.connect').trigger('click');
    const dialog = w.findAll('dialog').find(d => (d.element as HTMLDialogElement).open)!;
    expect(dialog.get('[role="alert"]').text()).toBe('openwa.publish_warning');
    useChannelsStore().save(yasaku.id, { id: 'chn_1', provider: 'openwa', status: 'waiting', baseUrl: 'https://openwa.example.id', deviceId: 'dev_a', webhookUrl: 'https://x/hooks', rejected24h: 0, rejectedLastHour: 0 });
    await flushPromises();
    expect(w.findAll('section [role="alert"]').map(a => a.text())).toContain('openwa.publish_warning');
  });

  it('hides connect actions without agent:write and says a never-published agent needs publishing', async () => {
    seed({ ...yasaku, publishedVersion: 0 }, ['agent:read']);
    const w = await mountPage(ChannelsPage);
    expect(w.text()).toContain('channels.read_only');
    expect(w.text()).toContain('openwa.needs_publish');
    expect(w.findAll('button').some(b => b.text().includes('openwa.connect'))).toBe(false);
  });
});
