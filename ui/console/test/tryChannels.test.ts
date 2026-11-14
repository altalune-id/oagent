// @vitest-environment happy-dom
import { create } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { flushPromises } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { channelDefaults, connectionPB, fakeChannelClient } from '~~/test/fixtures/channels';
import { fakeRuntimeClients } from '~~/test/fixtures/runtime';
import { useToast } from '#base/components/ui/toast';
import { WhoamiResponseSchema } from '#gen/auth/v1/auth_pb';
import { RunSchema, RunStatus } from '#gen/thread/v1/thread_pb';
import { mountPage } from '#test/mount-page';
import { runtimeConfig, setClient, setKnownKeys, setRoute } from '#test/nuxt-globals';
import ChannelsPage from '~/pages/o/[org]/projects/[project]/agents/[agent]/channels.vue';
import TryPage from '~/pages/o/[org]/projects/[project]/agents/[agent]/try.vue';
import { useChannelsStore } from '~/stores/channels';
import { useOrgsStore } from '~/stores/orgs';
import { useProjectsStore } from '~/stores/projects';

const orgId = 'B6rTn1YkM8wQz3XpL5vGd';
const projectId = 'x6MH9WPWjktMwCBKHy4pF';
const yasaku = { id: 'Y4sKu8RmT2pLq6WnX9cVb', projectId, slug: 'yasaku-keuangan', name: 'Yasaku Keuangan', description: '', publishedVersion: 4 };
const draftConfig = { instructions: 'Halo', providerId: 'Pv6tQm2ZxR8wKp3VbNc4K', model: 'openai/gpt-4.1-mini', helperModel: '', temperature: 0.3, maxSteps: 12, maxCostCents: 20, mcpServers: [], providerMissing: false, tools: [] as { name: string, requireApproval: boolean }[] };

function seed(agent: Record<string, unknown> = yasaku, permissions = ['agent:read', 'agent:write'], tools = draftConfig.tools, draft: Record<string, unknown> = {}) {
  useOrgsStore().items = [{ id: orgId, slug: 'acme', name: 'Acme', role: 'owner', permissions: [] }];
  useProjectsStore().items = [{ id: projectId, orgId, slug: 'rumah', name: 'Rumah Tangga' }];
  setRoute({ path: `/o/acme/projects/rumah/agents/${agent.slug}/try`, params: { org: 'acme', project: 'rumah', agent: agent.slug as string } });
  setClient('$authClient', { whoami: vi.fn().mockResolvedValue(create(WhoamiResponseSchema, { name: 'Budi', permissions })) });
  setClient('$agentClient', {
    listAgents: vi.fn().mockResolvedValue({ agents: [agent] }),
    getAgent: vi.fn().mockResolvedValue({ agent: { ...agent, draft: { ...draftConfig, tools, ...draft }, unpublishedChanges: true } }),
    listAgentVersions: vi.fn().mockResolvedValue({ versions: [] }),
  });
}

const button = (w: Awaited<ReturnType<typeof mountPage>>, text: string) => w.findAll('button').find(b => b.text().includes(text))!;

beforeEach(() => {
  localStorage.clear();
  setActivePinia(createPinia());
  fakeRuntimeClients();
  vi.stubGlobal('definePageMeta', vi.fn());
});

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe('try it page', () => {
  const runId = 'Rn2wQ8mX4vZ9tB3nC7kPq';
  const events = [
    { type: 'RUN_STARTED', runId },
    { type: 'TOOL_CALL_START', toolCallId: 'c1', toolCallName: 'yasaku__list_wallets' },
    { type: 'TOOL_CALL_ARGS', toolCallId: 'c1', delta: '{}' },
    { type: 'TOOL_CALL_RESULT', toolCallId: 'c1', content: '[]' },
    { type: 'TEXT_MESSAGE_CONTENT', messageId: 'm1', delta: 'Siap, sudah saya catat.' },
    { type: 'TEXT_MESSAGE_END', messageId: 'm1' },
    { type: 'RUN_FINISHED', runId },
  ];

  function streamOf(list: Record<string, unknown>[], open = false) {
    let push: (e: Record<string, unknown>) => void = () => {};
    const body = new ReadableStream<Uint8Array>({
      start(c) {
        const send = (e: Record<string, unknown>) => c.enqueue(new TextEncoder().encode(`data: ${JSON.stringify(e)}\n\n`));
        list.forEach(send);
        push = (e) => {
          send(e);
          c.close();
        };
        if (!open) {
          c.close();
        }
      },
    });
    return { body, push: (e: Record<string, unknown>) => push(e) };
  }

  function routes(run: () => { ok: boolean, status?: number, body?: unknown, json?: () => Promise<unknown> }) {
    const fetcher = vi.fn(async (url: string) => (url.endsWith('/cancel') ? { ok: true, status: 202, json: async () => ({}) } : run()));
    vi.stubGlobal('fetch', fetcher);
    const getRun = vi.fn().mockResolvedValue({ run: create(RunSchema, { id: runId, threadId: 'Th7sK2pQ9mXr4StV8nBcW', status: RunStatus.COMPLETED, agentVersionKind: 'draft', usage: { inputTokens: 300n, outputTokens: 12n }, costMicros: 4200n }) });
    setClient('$threadClient', { getRun });
    return { fetcher, getRun };
  }

  it('starts empty on the draft, with no sample prompts, scenarios, photo button or preview badge', async () => {
    seed();
    const w = await mountPage(TryPage);
    expect(w.get('h1').text()).toBe('try.title');
    expect(w.text()).toContain('try.empty');
    const tabs = w.findAll('label.tab');
    expect(tabs.map(l => l.text())).toEqual(['try.draft', 'try.live_legacy|{"n":4}']);
    expect(tabs[1]!.get('input').attributes('disabled')).toBeDefined();
    expect(tabs[1]!.classes()).toContain('tab-disabled');
    expect(w.text()).toContain('try.live_legacy_hint|{"n":4}');
    expect(w.get('aside').text()).toContain('try.phase_idle');
    expect(w.findAll('li button')).toHaveLength(0);
    expect(w.find('aside select').exists()).toBe(false);
    expect(w.find('input[type="file"]').exists()).toBe(false);
    expect(w.text()).not.toContain('preview.badge');
    const summary = w.get('details summary');
    expect([summary.attributes('role'), summary.find('[role="status"]').exists()]).toEqual([undefined, true]);
  });

  it('sends with Enter to the console Try it route and streams the reply, then shows the draft run and its usage', async () => {
    seed();
    const { fetcher, getRun } = routes(() => ({ ok: true, status: 200, body: streamOf(events).body }));
    const w = await mountPage(TryPage);
    const box = w.get('textarea');
    await box.setValue('beli kopi 40rb');
    await box.trigger('keydown', { key: 'Enter', shiftKey: true });
    expect(fetcher).not.toHaveBeenCalled();
    await box.trigger('keydown', { key: 'Enter' });
    await vi.waitFor(() => expect(w.get('aside').text()).toContain('runstatus.completed'));
    const [url, init] = fetcher.mock.calls[0]! as unknown as [string, RequestInit];
    expect([url, init.method, init.credentials]).toEqual([`/console/agents/${yasaku.id}/try/runs`, 'POST', 'same-origin']);
    expect(JSON.parse(init.body as string).messages[0]).toMatchObject({ role: 'user', content: 'beli kopi 40rb' });
    const log = w.get('[role="log"]');
    expect(log.text()).toContain('beli kopi 40rb');
    expect(log.text()).toContain('Siap, sudah saya catat.');
    expect(log.text()).toContain('yasaku__list_wallets');
    expect(log.get('.badge-ghost').text()).toBe('run.draft');
    expect(getRun).toHaveBeenCalledWith({ runId });
    expect(w.get('aside').text()).toContain('312');
    expect(w.findAll('aside tbody tr').map(r => r.findAll('td')[1]!.text())).toEqual(['try.tool_done']);
    expect(w.get('[data-testid="draft-note"]').text()).toBe('try.empty');
    await button(w, 'try.new_chat').trigger('click');
    expect(w.text()).toContain('try.empty');
    expect(w.find('[data-testid="draft-note"]').exists()).toBe(false);
  });

  it('reopens the last Try it conversation when you come back, and New chat starts and remembers a fresh one', async () => {
    seed();
    localStorage.setItem(`oagent_try_thread:${yasaku.id}`, 'thread-old');
    const { fetcher } = routes(() => ({ ok: true, status: 200, json: async () => ({ messages: [{ seq: 1, message: { role: 'user', content: 'beli kopi' } }, { seq: 2, message: { role: 'assistant', content: 'Sudah dicatat.' } }], next_after: 2 }) }));
    const w = await mountPage(TryPage);
    await vi.waitFor(() => expect(w.find('[role="log"]').exists()).toBe(true));
    expect(fetcher.mock.calls[0]![0]).toBe(`/console/agents/${yasaku.id}/try/threads/thread-old/messages?after=0&limit=200`);
    expect(w.get('[role="log"]').text()).toContain('Sudah dicatat.');
    expect(w.findAll('[role="log"] time')).toHaveLength(0);
    await button(w, 'try.new_chat').trigger('click');
    expect(w.text()).toContain('try.empty');
    expect(localStorage.getItem(`oagent_try_thread:${yasaku.id}`)).toBe('');
  });

  it('shows Stop while replying, Stopping after it, and Stopped by you only once the run says cancelled', async () => {
    seed();
    const s = streamOf([{ type: 'RUN_STARTED', runId }], true);
    const { fetcher, getRun } = routes(() => ({ ok: true, status: 200, body: s.body }));
    getRun.mockResolvedValue({ run: create(RunSchema, { id: runId, status: RunStatus.CANCELLED, agentVersionKind: 'draft' }) });
    const w = await mountPage(TryPage);
    await w.get('textarea').setValue('halo');
    await w.get('form').trigger('submit');
    await vi.waitFor(() => expect(button(w, 'try.stop')?.exists()).toBe(true));
    expect(w.get('textarea').attributes('placeholder')).toBe('try.replying');
    await new Promise(r => setTimeout(r, 0));
    await button(w, 'try.stop').trigger('click');
    await flushPromises();
    expect(fetcher.mock.calls.map(c => c[0])).toContain(`/console/agents/${yasaku.id}/try/runs/${runId}/cancel`);
    expect(button(w, 'try.phase_stopping').attributes('disabled')).toBeDefined();
    expect(w.text()).not.toContain('try.stopped');
    s.push({ type: 'RUN_ERROR', code: 'cancelled' });
    await vi.waitFor(() => expect(w.text()).toContain('try.stopped'));
  });

  const unattended = { knowledgeSourceIds: ['Ks1Upl0adQ8sWx3YcLm7D'], mcpServers: [{ serverId: 'Ms2bVn7QxK4pRt9KwZc3H', tools: [{ name: 'list_wallets', requireApproval: false, freeWhen: [] }], missing: false }] };

  it('warns without blocking the send', async () => {
    seed(yasaku, undefined, undefined, unattended);
    routes(() => ({ ok: true, status: 200, body: streamOf(events).body }));
    const w = await mountPage(TryPage);
    await flushPromises();
    expect(w.findAll('[role="status"]').map(s => s.text()).find(s => s.includes('publish.warning_unapproved_tool'))).toContain('publish.warning_unapproved_tool|{"tool":"yasaku__list_wallets","n":"0"}');
    expect(w.findAll('[role="status"]').map(s => s.text()).find(s => s.includes('publish.warning_unapproved_tool'))).toContain('"n":"0"} try.open_configure');
    expect(w.text()).not.toContain('try.cant_run');
    expect(w.get('textarea').attributes('disabled')).toBeUndefined();
  });

  it('shows the AGT007 unapproved_tool refusal of a send as the app error naming the tool', async () => {
    seed(yasaku, undefined, undefined, unattended);
    setKnownKeys('apperror.AGT007_unapproved_tool');
    routes(() => ({ ok: false, status: 400, json: async () => ({ code: 'AGT007', message: 'refused', request_id: 'req1', meta: { reason: 'unapproved_tool', name: 'yasaku__list_wallets' } }) }));
    const w = await mountPage(TryPage);
    await w.get('textarea').setValue('halo');
    await w.get('form').trigger('submit');
    await vi.waitFor(() => expect(w.text()).toContain('apperror.AGT007_unapproved_tool|{"name":"yasaku__list_wallets"}'));
    expect(w.text()).not.toContain('Siap, sudah saya catat.');
  });

  it('shows a refused turn as the app error, and Retry after a failed run', async () => {
    seed();
    setKnownKeys('apperror.RUN007');
    const refusal = { ok: false, status: 429, json: async () => ({ code: 'RUN007', message: 'busy', request_id: 'req1' }) };
    const failed = [{ type: 'RUN_STARTED', runId }, { type: 'RUN_ERROR', code: 'runtime_error' }];
    const replies = [() => refusal, () => ({ ok: true, status: 200, body: streamOf(failed).body }), () => ({ ok: true, status: 200, body: streamOf(events).body })];
    const { getRun } = routes(() => replies.shift()!());
    getRun.mockResolvedValueOnce({ run: create(RunSchema, { id: runId, status: RunStatus.FAILED, errorCode: 'RUN004', agentVersionKind: 'draft' }) });
    const w = await mountPage(TryPage);
    await w.get('textarea').setValue('halo');
    await w.get('form').trigger('submit');
    await vi.waitFor(() => expect(w.text()).toContain('apperror.RUN007'));
    expect(w.get('aside').text()).toContain('runstatus.failed');
    await w.get('textarea').setValue('halo lagi');
    await w.get('form').trigger('submit');
    await vi.waitFor(() => expect(w.text()).toContain('try.model_failed'));
    expect(w.text()).not.toContain('apperror.RUN007');
    await button(w, 'common.retry').trigger('click');
    await vi.waitFor(() => expect(w.get('[role="log"]').text()).toContain('Siap, sudah saya catat.'));
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

  it('says on the WhatsApp section whether voice notes are transcribed, naming the set reply when they are off', async () => {
    const offPage = await page(vision);
    expect(offPage.w.text()).toContain('openwa.voice_off|{"reply":"channels.reply_voice"}');
    expect(offPage.w.text()).not.toContain('openwa.voice_on');
    offPage.w.unmount();
    setActivePinia(createPinia());
    fakeRuntimeClients();
    const onPage = await page({ ...yasaku, draft: { ...draftConfig, transcriptionModel: { providerId: 'Pv9pHs4YvT7kMq7WcXe2B', model: 'whisper-large-v3', providerMissing: false } } });
    expect(onPage.w.text()).toContain('openwa.voice_on|{"model":"whisper-large-v3"}');
    expect(onPage.w.text()).not.toContain('openwa.voice_after_publish');
    expect(onPage.w.text()).not.toContain('openwa.voice_off');
    onPage.w.unmount();
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

  it('offers the new voice note reply when voice notes are on and the connection still has the old default, saving only that reply', async () => {
    const old = 'Sorry, I can\'t listen to voice messages yet. Please type your message.';
    const voiceOn = { ...yasaku, draft: { ...draftConfig, transcriptionModel: { providerId: 'Pv9pHs4YvT7kMq7WcXe2B', model: 'whisper-large-v3', providerMissing: false } } };
    const { w, client } = await page(voiceOn, writer, [ofYasaku({ replyVoice: old, replyFailure: 'Maaf.' })]);
    const section = w.findAll('section').find(s => s.get('h2').text() === 'channels.replies')!;
    expect(section.get('[role="note"]').text()).toContain('channels.voice_reply_old|{"reply":"channels.reply_voice"}');
    await section.findAll('textarea')[0]!.setValue('Unsaved edit');
    await section.findAll('button').find(b => b.text() === 'channels.voice_reply_update')!.trigger('click');
    await flushPromises();
    expect(client.updateConnection).toHaveBeenLastCalledWith({ id: 'Ch8fK2pQ7mXr9Lt4WvN3c', replyVoice: channelDefaults.replyVoice });
    expect(section.find('[role="note"]').exists()).toBe(false);
    expect((section.findAll('textarea')[0]!.element as HTMLTextAreaElement).value).toBe('Unsaved edit');
    expect(useToast().toasts.at(-1)?.text).toBe('channels.voice_reply_updated');
    w.unmount();
    setActivePinia(createPinia());
    fakeRuntimeClients();
    const off = await page(vision, writer, [ofYasaku({ replyVoice: old })]);
    expect(off.w.text()).not.toContain('channels.voice_reply_old');
    off.w.unmount();
    setActivePinia(createPinia());
    fakeRuntimeClients();
    const reader = await page(voiceOn, ['agent:read', 'channel:read'], [ofYasaku({ replyVoice: old })]);
    expect(reader.w.text()).toContain('channels.voice_reply_old');
    expect(reader.w.findAll('button').some(b => b.text() === 'channels.voice_reply_update')).toBe(false);
    reader.w.unmount();
    setActivePinia(createPinia());
    fakeRuntimeClients();
    const draft = await page({ ...voiceOn, unpublishedChanges: true }, writer, [ofYasaku({ replyVoice: old })]);
    expect(draft.w.text()).toContain('openwa.voice_after_publish');
    const note = draft.w.findAll('p[role="note"]').find(p => p.text().includes('openwa.voice_after_publish'))!;
    expect(note.element.textContent!.replace(/\s+/g, ' ').trim()).toBe('openwa.voice_on|{"model":"whisper-large-v3"} openwa.voice_after_publish configure.voice');
    expect(note.element.textContent).toMatch(/openwa\.voice_after_publish\s+configure\.voice/);
    expect(draft.w.text()).not.toContain('channels.voice_reply_old');
    draft.w.unmount();
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

  it('drops the quiet range from the hints and the check when the server sends no limit', async () => {
    seed(vision, writer);
    channelsAt(vision);
    fakeChannelClient([ofYasaku()]).client.getChannelSettings.mockResolvedValue({ ...channelDefaults, maxQuietMs: 0 });
    const w = await mountPage(ChannelsPage, { attachTo: document.body });
    await flushPromises();
    const section = w.findAll('section').find(s => s.get('h2').text() === 'channels.windows')!;
    expect(section.text()).toContain('channels.quiet_window_hint_open');
    expect(section.text()).toContain('channels.quiet_max_hint_open');
    expect(section.text()).not.toContain('{"max"');
    const [quiet, max] = section.findAll('input');
    await quiet!.setValue('16');
    await max!.setValue('20');
    await section.get('form').trigger('submit');
    expect(quiet!.attributes('aria-invalid')).toBe('false');
    expect(section.text()).not.toContain('channels.error_quiet');
    w.unmount();
  });

  it('edits the quiet window and its maximum in seconds, up to the server\'s limit, the window no longer than the maximum', async () => {
    const { w, client } = await page(vision, writer, [ofYasaku()]);
    const section = w.findAll('section').find(s => s.get('h2').text() === 'channels.windows')!;
    const [quiet, max] = section.findAll('input');
    expect([(quiet!.element as HTMLInputElement).value, (max!.element as HTMLInputElement).value]).toEqual(['3', '10']);
    expect(section.text()).toContain('channels.late_cutoff_value|{"n":30}');
    expect(section.text()).toContain('channels.quiet_window_hint|{"max":15}');
    expect(section.text()).toContain('channels.quiet_max_hint|{"max":15}');
    await quiet!.setValue('16');
    await section.get('form').trigger('submit');
    expect(quiet!.attributes('aria-invalid')).toBe('true');
    expect(section.text()).toContain('channels.error_quiet_range|{"max":15}');
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
