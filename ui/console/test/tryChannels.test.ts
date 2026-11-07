// @vitest-environment happy-dom
import { create } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { flushPromises } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { connectionPB, fakeChannelClient } from '~~/test/fixtures/channels';
import { fakeRuntimeClients } from '~~/test/fixtures/runtime';
import { useToast } from '#base/components/ui/toast';
import { WhoamiResponseSchema } from '#gen/auth/v1/auth_pb';
import { mountPage } from '#test/mount-page';
import { runtimeConfig, setClient, setRoute } from '#test/nuxt-globals';
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
  const vision = { ...yasaku, draft: { ...draftConfig, providerId: 'Pv6tQm2ZxR8wKp3VbNc4K', model: 'openai/gpt-4.1-mini' } };
  const writer = ['agent:read', 'channel:read', 'channel:write'];
  const dialog = (w: Awaited<ReturnType<typeof mountPage>>) => w.findAll('dialog').find(d => (d.element as HTMLDialogElement).open)!;
  const dialogButton = (w: Awaited<ReturnType<typeof mountPage>>, text: string) => dialog(w).findAll('button').find(b => b.text() === text)!;
  const channelsAt = (agent: Record<string, unknown>) => setRoute({ path: `/o/acme/projects/rumah/agents/${agent.slug}/channels`, params: { org: 'acme', project: 'rumah', agent: agent.slug as string } });
  const ofYasaku = (over: Partial<Parameters<typeof connectionPB>[0]> = {}) => connectionPB({ agentId: yasaku.id, ...over });

  async function page(agent: Record<string, unknown> = vision, permissions = writer, seedConnections: ReturnType<typeof connectionPB>[] = []) {
    seed(agent, permissions);
    channelsAt(agent);
    const fake = fakeChannelClient(seedConnections);
    const w = await mountPage(ChannelsPage, { attachTo: document.body });
    await flushPromises();
    return { w, ...fake };
  }

  it('connects WhatsApp: before you start, then details and key, then the server\'s webhook URL, the pasted secret and a check', async () => {
    vi.useFakeTimers();
    const { w, client, set } = await page();
    expect(w.find('h1 + .badge').exists()).toBe(false);
    expect(w.findAll('section h2').map(h => h.text())).toEqual(['openwa.title', 'channels.provider_whatsapp_business', 'apiaccess.title', 'widget.title']);
    expect(w.text()).toContain('openwa.status_none');
    await button(w, 'openwa.connect').trigger('click');
    expect(dialog(w).get('li.step[aria-current="step"]').text()).toBe('openwa.step_before');
    expect(dialog(w).text()).not.toContain('openwa.webhook_url');
    await dialogButton(w, 'openwa.next').trigger('click');
    const field = (label: string) => dialog(w).findAll('label').find(l => l.text().startsWith(label))!.get('input');
    expect((field('openwa.base_url').element as HTMLInputElement).value).toBe('https://openwa.altalune.id');
    const hint = dialog(w).get('p[id$="-ws-hint"]');
    expect(hint.text()).toBe('openwa.workspace_hint');
    expect(field('openwa.org').attributes('aria-describedby')).toContain(hint.attributes('id'));
    expect(field('openwa.project').attributes('aria-describedby')).toContain(hint.attributes('id'));
    await field('openwa.org').setValue('altalune');
    await field('openwa.project').setValue('rumah');
    await field('openwa.device_id').setValue('dev_8fK2pQ7mXr9Lt4Wv');
    await field('openwa.device_key').setValue('owk_1');
    await dialogButton(w, 'openwa.save').trigger('click');
    await flushPromises();
    expect(client.createConnection).toHaveBeenCalledWith(expect.objectContaining({ projectId, agentId: yasaku.id, deviceId: 'dev_8fK2pQ7mXr9Lt4Wv', deviceKey: 'owk_1' }));
    expect(client.createConnection.mock.calls[0]![0]).not.toHaveProperty('signingSecret');
    expect(dialog(w).get('li.step[aria-current="step"]').text()).toBe('openwa.step_webhook');
    expect(dialog(w).text()).toContain('https://oagent.altalune.id/hooks/openwa/Ch');
    expect(w.text()).toContain('openwa.status_needs_secret');
    expect(w.text()).not.toContain('openwa.status_waiting');
    expect(dialog(w).text()).not.toContain('openwa.check_help');
    await dialogButton(w, 'openwa.save_secret').trigger('click');
    expect(dialog(w).text()).toContain('openwa.secret_required');
    await field('openwa.secret').setValue('whsec_1');
    await dialogButton(w, 'openwa.save_secret').trigger('click');
    await vi.advanceTimersByTimeAsync(0);
    expect(client.setConnectionSecret).toHaveBeenCalledWith(expect.objectContaining({ signingSecret: 'whsec_1', dropPrevious: false }));
    expect(w.text()).toContain('openwa.status_waiting');
    expect(dialog(w).text()).toContain('openwa.checking');
    set(useChannelsStore().connection(yasaku.id)!.id, { status: 2 });
    await vi.advanceTimersByTimeAsync(4_000);
    await flushPromises();
    expect(dialog(w).text()).toContain('openwa.result_connected');
    await dialogButton(w, 'common.done').trigger('click');
    expect(w.text()).toContain('openwa.status_connected');
    expect(w.text()).toContain('altalune / rumah');
    w.unmount();
  });

  it('names the device on a mismatch and opens the device step to change device id and key together', async () => {
    const { w, client } = await page(vision, writer, [ofYasaku({ status: 3 })]);
    const alert = w.get('[role="alert"]');
    expect(alert.text()).toContain('openwa.problem_device_mismatch|{"device":"dev_8fK2pQ7mXr9Lt4Wv"}');
    await alert.findAll('button').find(b => b.text() === 'openwa.change_device')!.trigger('click');
    await flushPromises();
    expect(dialog(w).get('li.step[aria-current="step"]').text()).toBe('openwa.step_device');
    const field = (label: string) => dialog(w).findAll('label').find(l => l.text().startsWith(label))!.get('input');
    expect((field('openwa.device_id').element as HTMLInputElement).value).toBe('dev_8fK2pQ7mXr9Lt4Wv');
    await field('openwa.device_id').setValue('dev_NewDevice000000a');
    await dialogButton(w, 'openwa.save').trigger('click');
    await flushPromises();
    expect(dialog(w).text()).toContain('openwa.key_required');
    expect(client.updateConnection).not.toHaveBeenCalled();
    await field('openwa.device_key').setValue('owk_new_device_key');
    await dialogButton(w, 'openwa.save').trigger('click');
    await flushPromises();
    expect(client.updateConnection).toHaveBeenCalledWith(expect.objectContaining({ deviceId: 'dev_NewDevice000000a', deviceKey: 'owk_new_device_key' }));
    w.unmount();
  });

  it('names a refused key and a wrong device, and shows a bad signature as a hint with Replace secret, not as a status', async () => {
    const { w, client } = await page(vision, writer, [ofYasaku({ status: 4 })]);
    expect(w.get('[role="alert"]').text()).toContain('openwa.problem_key_refused');
    await w.get('[role="alert"]').findAll('button').find(b => b.text() === 'openwa.replace_key')!.trigger('click');
    expect(dialog(w).text()).toContain('openwa.new_key');
    w.unmount();
    setActivePinia(createPinia());
    fakeRuntimeClients();
    const bad = await page(vision, writer, [ofYasaku({ lastBadSignatureAt: timestampFromDate(new Date('2026-11-07T09:30:00Z')) })]);
    expect(bad.w.text()).toContain('openwa.status_connected');
    const hint = bad.w.findAll('[role="note"]').find(n => n.text().includes('openwa.bad_signature_seen'))!;
    await hint.findAll('button').find(b => b.text() === 'openwa.replace_secret')!.trigger('click');
    expect(dialog(bad.w).text()).toContain('openwa.rotation_note');
    expect((dialog(bad.w).get('input[type="checkbox"]').element as HTMLInputElement).checked).toBe(false);
    await dialog(bad.w).get('input[type="password"]').setValue('whsec_2');
    await dialogButton(bad.w, 'common.save').trigger('click');
    await flushPromises();
    expect(bad.client.setConnectionSecret).toHaveBeenCalledWith({ id: 'Ch8fK2pQ7mXr9Lt4WvN3c', signingSecret: 'whsec_2', dropPrevious: false });
    expect(bad.w.text()).toContain('openwa.secondary_set');
    await bad.w.findAll('button').find(b => b.text() === 'openwa.replace_secret')!.trigger('click');
    await dialog(bad.w).get('input[type="checkbox"]').setValue(true);
    await dialog(bad.w).get('input[type="password"]').setValue('whsec_3');
    await dialogButton(bad.w, 'common.save').trigger('click');
    await flushPromises();
    expect(bad.client.setConnectionSecret).toHaveBeenLastCalledWith({ id: 'Ch8fK2pQ7mXr9Lt4WvN3c', signingSecret: 'whsec_3', dropPrevious: true });
    expect(bad.w.text()).not.toContain('openwa.secondary_set');
    expect(client.setConnectionSecret).not.toHaveBeenCalled();
    bad.w.unmount();
  });

  it('continues setup while waiting, and disconnects through the confirm', async () => {
    const { w, client } = await page(vision, writer, [ofYasaku({ status: 1, lastResultAt: undefined })]);
    expect(w.text()).toContain('openwa.status_waiting');
    await button(w, 'openwa.continue_setup').trigger('click');
    expect(dialog(w).get('li.step[aria-current="step"]').text()).toBe('openwa.step_webhook');
    expect(dialog(w).text()).toContain('openwa.check_help');
    await dialogButton(w, 'openwa.finish_later').trigger('click');
    await button(w, 'openwa.disconnect').trigger('click');
    await flushPromises();
    await dialogButton(w, 'openwa.disconnect').trigger('click');
    await flushPromises();
    expect(client.deleteConnection).toHaveBeenCalledWith({ id: 'Ch8fK2pQ7mXr9Lt4WvN3c' });
    expect(w.text()).toContain('openwa.status_none');
    w.unmount();
  });

  it('warns when the agent\'s model can\'t read photos, and not when it can', async () => {
    const plain = await page({ ...yasaku, draft: { ...draftConfig, providerId: 'Pv9pHs4YvT7kMq7WcXe2B', model: 'llama-3.3-70b-versatile' } });
    expect(plain.w.text()).toContain('openwa.no_vision|{"model":"llama-3.3-70b-versatile"}');
    plain.w.unmount();
    setActivePinia(createPinia());
    fakeRuntimeClients();
    const sees = await page(vision);
    expect(sees.w.text()).not.toContain('openwa.no_vision');
    sees.w.unmount();
    setActivePinia(createPinia());
    fakeRuntimeClients();
    const none = await page({ ...yasaku, draft: { ...draftConfig, providerId: '', model: '' } });
    expect(none.w.text()).not.toContain('openwa.no_vision');
    none.w.unmount();
  });

  it('makes no approval promise on this build, and says messages get no reply while unpublished', async () => {
    const { w } = await page({ ...vision, unpublishedChanges: true }, writer, [ofYasaku()]);
    expect(w.text()).not.toContain('channels.writes_note');
    expect(w.text()).toContain('openwa.publish_warning');
    w.unmount();
  });

  it('builds the API endpoint from the server-filled apiUrl, not from the webhook URL', async () => {
    runtimeConfig.public.apiUrl = '/oa/api';
    const based = await page(vision, writer, [ofYasaku({ webhookUrl: 'https://elsewhere.example.id/hooks/openwa/Ch8fK2pQ7mXr9Lt4WvN3c' })]);
    expect(based.w.text()).toContain(`${window.location.origin}/oa/api/v1/agents/${yasaku.id}/runs`);
    expect(based.w.text()).not.toContain('elsewhere.example.id/api');
    based.w.unmount();
    setActivePinia(createPinia());
    fakeRuntimeClients();
    runtimeConfig.public.apiUrl = 'https://api.example.id/api/';
    const absolute = await page(vision, writer, []);
    expect(absolute.w.text()).toContain(`https://api.example.id/api/v1/agents/${yasaku.id}/runs`);
    absolute.w.unmount();
    setActivePinia(createPinia());
    fakeRuntimeClients();
    runtimeConfig.public.apiUrl = '';
    const none = await page(vision, writer, []);
    expect(none.w.text()).toContain(`${window.location.origin}/api/v1/agents/${yasaku.id}/runs`);
    none.w.unmount();
  });

  it('saves the set replies under one visible heading, refusing a blank one', async () => {
    const { w, client } = await page(vision, writer, [ofYasaku()]);
    const section = w.findAll('section').find(s => s.get('h2').text() === 'channels.replies')!;
    expect(section.get('legend').classes()).toContain('sr-only');
    expect(w.findAll('h2, legend:not(.sr-only)').filter(h => h.text() === 'channels.replies')).toHaveLength(1);
    const areas = section.findAll('textarea');
    await areas[0]!.setValue(' ');
    await section.findAll('button').find(b => b.text() === 'channels.save_replies')!.trigger('click');
    expect(section.text()).toContain('channels.error_reply_required');
    expect(client.updateConnection).not.toHaveBeenCalled();
    await areas[0]!.setValue(' Maaf, coba lagi nanti. ');
    await section.findAll('button').find(b => b.text() === 'channels.save_replies')!.trigger('click');
    await flushPromises();
    expect(client.updateConnection).toHaveBeenCalledWith(expect.objectContaining({ id: 'Ch8fK2pQ7mXr9Lt4WvN3c', replyFailure: 'Maaf, coba lagi nanti.' }));
    w.unmount();
  });

  it('edits the quiet window and its maximum in seconds, 0-10 s, the window no longer than the maximum', async () => {
    const { w, client } = await page(vision, writer, [ofYasaku()]);
    const section = w.findAll('section').find(s => s.get('h2').text() === 'channels.windows')!;
    const [quiet, max] = section.findAll('input');
    expect([(quiet!.element as HTMLInputElement).value, (max!.element as HTMLInputElement).value]).toEqual(['3', '10']);
    expect(section.text()).toContain('channels.late_cutoff_value|{"n":30}');
    await quiet!.setValue('12');
    await section.get('form').trigger('submit');
    expect(quiet!.attributes('aria-invalid')).toBe('true');
    expect(section.text()).toContain('channels.error_quiet_range');
    await quiet!.setValue('6');
    await max!.setValue('4');
    await section.get('form').trigger('submit');
    expect(section.text()).toContain('channels.error_quiet_order');
    expect(client.updateConnection).not.toHaveBeenCalled();
    await quiet!.setValue('2.5');
    await max!.setValue('8');
    await section.get('form').trigger('submit');
    await flushPromises();
    expect(client.updateConnection).toHaveBeenCalledWith({ id: 'Ch8fK2pQ7mXr9Lt4WvN3c', quietWindowMs: 2_500, quietMaxMs: 8_000 });
    w.unmount();
  });

  it('turns the number off only after a confirm, back on at once, and shows Off beside the reported status while off', async () => {
    const { w, client } = await page(vision, writer, [ofYasaku()]);
    const box = () => w.get('[data-testid="channel-switch"] input[role="switch"]');
    expect((box().element as HTMLInputElement).checked).toBe(true);
    expect(w.get('[data-testid="channel-switch"]').text()).toContain('openwa.on_hint');
    await box().setValue(false);
    expect(dialog(w).text()).toContain('openwa.turn_off_body');
    expect((box().element as HTMLInputElement).checked).toBe(true);
    await dialogButton(w, 'common.cancel').trigger('click');
    await flushPromises();
    expect(client.updateConnection).not.toHaveBeenCalled();
    await box().setValue(false);
    await dialogButton(w, 'openwa.turn_off').trigger('click');
    await flushPromises();
    expect(client.updateConnection).toHaveBeenCalledWith({ id: 'Ch8fK2pQ7mXr9Lt4WvN3c', enabled: false });
    expect((box().element as HTMLInputElement).checked).toBe(false);
    expect(w.get('[data-testid="channel-switch"]').text()).toContain('openwa.off_hint');
    expect(w.text()).toContain('openwa.status_connected');
    expect(w.get('[data-testid="status-off"]').text()).toBe('· openwa.switch_off');
    expect(useToast().toasts.at(-1)?.text).toBe('openwa.turned_off');
    await box().setValue(true);
    await flushPromises();
    expect(client.updateConnection).toHaveBeenLastCalledWith({ id: 'Ch8fK2pQ7mXr9Lt4WvN3c', enabled: true });
    expect(w.text()).toContain('openwa.status_connected');
    expect(w.find('[data-testid="status-off"]').exists()).toBe(false);
    expect(useToast().toasts.at(-1)?.text).toBe('openwa.turned_on');
    w.unmount();
  });

  it('shows a failed switch as an error and keeps the state', async () => {
    const { w, client } = await page(vision, writer, [ofYasaku({ enabled: false })]);
    client.updateConnection.mockRejectedValueOnce(new Error('offline'));
    await w.get('[data-testid="channel-switch"] input[role="switch"]').setValue(true);
    await flushPromises();
    expect(useToast().toasts.at(-1)).toMatchObject({ kind: 'error', text: 'error.save_channel' });
    expect(w.get('[data-testid="channel-switch"]').text()).toContain('openwa.off_hint');
    w.unmount();
  });

  it('is read-only without channel:write', async () => {
    const { w } = await page(vision, ['agent:read', 'channel:read'], [ofYasaku()]);
    expect(w.text()).toContain('channels.read_only');
    expect(w.find('[data-testid="channel-switch"] input').exists()).toBe(false);
    expect(w.get('[data-testid="channel-switch"]').text()).toContain('openwa.switch_label: openwa.switch_on');
    expect(w.findAll('button').some(b => !b.element.closest('dialog') && ['openwa.disconnect', 'openwa.replace', 'channels.save_timing'].includes(b.text()))).toBe(false);
    expect(w.findAll('section').find(s => s.get('h2').text() === 'channels.windows')!.get('fieldset').attributes('disabled')).toBeDefined();
    w.unmount();
  });
});
