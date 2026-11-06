// @vitest-environment happy-dom
import { create } from '@bufbuild/protobuf';
import { flushPromises, RouterLinkStub } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { useToast } from '#base/components/ui/toast';
import { WhoamiResponseSchema } from '#gen/auth/v1/auth_pb';
import { RunSchema, RunStatus, ThreadMessageSchema, ThreadSchema } from '#gen/thread/v1/thread_pb';
import { mountPage } from '#test/mount-page';
import { route, router, setClient, setRoute } from '#test/nuxt-globals';
import { useNow } from '~/composables/useNow';
import ThreadPage from '~/pages/o/[org]/projects/[project]/agents/[agent]/threads/[thread].vue';
import ThreadsPage from '~/pages/o/[org]/projects/[project]/agents/[agent]/threads/index.vue';
import ApprovalsPage from '~/pages/o/[org]/projects/[project]/approvals.vue';
import { useApprovalsStore } from '~/stores/approvals';
import { useEndUsersStore } from '~/stores/endUsers';
import { useOrgsStore } from '~/stores/orgs';
import { useProjectsStore } from '~/stores/projects';
import { useThreadsStore } from '~/stores/threads';

const orgId = 'B6rTn1YkM8wQz3XpL5vGd';
const projectId = 'x6MH9WPWjktMwCBKHy4pF';
const yasaku = { id: 'Y4sKu8RmT2pLq6WnX9cVb', projectId, slug: 'yasaku-keuangan', name: 'Yasaku Keuangan', description: '', publishedVersion: 4 };
const other = { ...yasaku, id: 'Ot8KqRm2LxW4pTn6Vc9Bd', slug: 'tagihan', name: 'Tagihan' };

function seed(path: string, params: Record<string, string>, permissions = ['agent:read', 'agent:write'], query: Record<string, string> = {}) {
  useOrgsStore().items = [{ id: orgId, slug: 'acme', name: 'Acme', role: 'owner', permissions: [] }];
  useProjectsStore().items = [{ id: projectId, orgId, slug: 'rumah', name: 'Rumah Tangga' }];
  setRoute({ path: `/o/acme/projects/rumah${path}`, params: { org: 'acme', project: 'rumah', ...params }, query });
  setClient('$authClient', { whoami: vi.fn().mockResolvedValue(create(WhoamiResponseSchema, { permissions })) });
  setClient('$agentClient', { listAgents: vi.fn().mockResolvedValue({ agents: [yasaku, other] }) });
}

const button = (w: { findAll: (s: string) => { text: () => string }[] }, text: string) => w.findAll('button').find(b => b.text().includes(text)) as unknown as { trigger: (e: string) => Promise<void> };

beforeEach(() => {
  setActivePinia(createPinia());
  vi.stubGlobal('definePageMeta', vi.fn());
});

afterEach(() => {
  vi.useRealTimers();
});

describe('threads page', () => {
  it('lists the sample conversations and filters by channel, status and words', async () => {
    seed('/agents/yasaku-keuangan/threads', { agent: 'yasaku-keuangan' });
    const w = await mountPage(ThreadsPage);
    expect(w.get('h1').text()).toBe('threads.title');
    expect(w.text()).toContain('threads.preview_note');
    expect(w.findAll('tbody tr')).toHaveLength(4);
    expect(w.get('tbody').text()).toContain('Sari: transfer 200rb ke BCA');
    expect(w.get('tbody').text()).toContain('threads.api_user|{"id":"cust_0001"}');
    const group = w.get('[role="group"]');
    expect(w.get(`#${group.attributes('aria-labelledby')}`).text()).toBe('threads.col_channel');
    const whatsapp = group.findAll('button')[0]!;
    await whatsapp.trigger('click');
    expect(whatsapp.attributes('aria-pressed')).toBe('true');
    expect(whatsapp.find('[data-icon="lucide:check"]').exists()).toBe(true);
    expect(w.findAll('tbody tr')).toHaveLength(2);
    await flushPromises();
    expect(router.replace).toHaveBeenLastCalledWith({ query: { channel: 'whatsapp' } });
    await w.get('input[type="search"]').setValue('zzz');
    expect(w.text()).toContain('threads.no_match');
    await button(w, 'threads.clear').trigger('click');
    expect(w.findAll('tbody tr')).toHaveLength(4);
    await w.get('select').setValue('cancelled');
    expect(w.findAll('tbody tr')).toHaveLength(1);
  });

  it('starts from the filter in the URL', async () => {
    seed('/agents/yasaku-keuangan/threads', { agent: 'yasaku-keuangan' }, ['agent:read'], { channel: 'api', q: 'makan' });
    const w = await mountPage(ThreadsPage);
    expect(w.findAll('tbody tr')).toHaveLength(1);
    expect((w.get('input[type="search"]').element as HTMLInputElement).value).toBe('makan');
    route.query = { channel: 'whatsapp' };
    await flushPromises();
    expect(w.findAll('tbody tr')).toHaveLength(2);
    expect((w.get('input[type="search"]').element as HTMLInputElement).value).toBe('');
  });

  it('starts empty for another agent with links to Try it and Channels', async () => {
    seed('/agents/tagihan/threads', { agent: 'tagihan' });
    const listThreads = vi.fn().mockResolvedValue({ threads: [], nextPageToken: '' });
    setClient('$threadClient', { listThreads });
    const w = await mountPage(ThreadsPage);
    expect(w.text()).toContain('threads.empty');
    expect(w.text()).not.toContain('threads.preview_note');
    expect(w.text()).not.toContain('preview.badge');
    expect(listThreads).toHaveBeenCalledWith(expect.objectContaining({ agentId: 'Ot8KqRm2LxW4pTn6Vc9Bd', pageToken: '' }));
  });
});

describe('thread view', () => {
  it('shows Cancelling and hides Cancel run after a cancel until the run settles, still polling', async () => {
    vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval'] });
    const threadId = 'Th3kP9wQ2mXr7LtV5nBcZ';
    const runId = 'Rn2wQ8mX4vL9tB3nC7kPq';
    const running = create(RunSchema, { id: runId, threadId, status: RunStatus.RUNNING, usage: {} });
    const thread = create(ThreadSchema, { id: threadId, projectId, agentId: other.id, kind: 'api', endUserLabel: 'API user ·…a91f', openRunStatus: RunStatus.RUNNING, runCount: 1n });
    const client = {
      listThreads: vi.fn().mockResolvedValue({ threads: [thread], nextPageToken: '' }),
      getThread: vi.fn().mockResolvedValue({ thread }),
      listRuns: vi.fn().mockResolvedValue({ runs: [running], nextPageToken: '' }),
      listThreadMessages: vi.fn().mockResolvedValueOnce({ messages: [create(ThreadMessageSchema, { seq: 1n, role: 'user', runId, message: { content: 'hi' } })] }).mockResolvedValue({ messages: [] }),
      getRun: vi.fn().mockResolvedValueOnce({ run: running }).mockResolvedValue({ run: create(RunSchema, { id: runId, threadId, status: RunStatus.CANCELLED, usage: {} }) }),
      cancelRun: vi.fn().mockResolvedValue({ run: running }),
    };
    setClient('$threadClient', client);
    seed(`/agents/tagihan/threads/${threadId}`, { agent: 'tagihan', thread: threadId });
    const w = await mountPage(ThreadPage);
    expect(w.text()).toContain('runstatus.running');
    await button(w, 'thread.cancel_run').trigger('click');
    await flushPromises();
    const dialog = w.findAll('dialog').find(d => d.text().includes('thread.cancel_body'))!;
    await dialog.findAll('button').find(b => b.text() === 'thread.cancel_run')!.trigger('click');
    await flushPromises();
    expect(client.cancelRun).toHaveBeenCalledWith({ runId });
    expect(w.text()).toContain('thread.cancelling');
    expect(w.findAll('button').some(b => b.text().includes('thread.cancel_run') && !b.element.closest('dialog'))).toBe(false);
    await vi.advanceTimersByTimeAsync(2_000);
    expect(w.text()).toContain('thread.cancelling');
    await vi.advanceTimersByTimeAsync(2_000);
    expect(client.getRun).toHaveBeenCalledTimes(2);
    expect(w.text()).toContain('runstatus.cancelled');
    expect(w.text()).not.toContain('thread.cancelling');
    w.unmount();
  });

  it('shows each speaker, run dividers, tool chips and decided approvals read-only, with the facts', async () => {
    vi.useFakeTimers({ now: new Date(2026, 10, 5, 12), toFake: ['Date'] });
    seed('/agents/yasaku-keuangan/threads/thr_grp3Kd9Q', { agent: 'yasaku-keuangan', thread: 'thr_grp3Kd9Q' });
    const w = await mountPage(ThreadPage, { attachTo: document.body });
    expect(w.get('h1').text()).toBe('Keuangan Rumah');
    expect(w.text()).toContain('thread.summary|{"runs":4,"cost":"< $0.01"}');
    expect(w.findAll('[title="$0.0054"]')).toHaveLength(2);
    const log = w.get('[role="log"]');
    expect(log.findAll('.chat-start .chat-header').map(h => h.text().split(' ')[0])).toEqual(['Budi', 'Sari', 'Sari']);
    expect(log.text()).toContain('thread.photo_placeholder');
    expect(log.findAllComponents(RouterLinkStub).map(l => l.props('to') as string).filter(to => to.includes('/runs/'))).toEqual(['/runs/run_6Hy2NfQs4TbW', '/runs/run_2Bn6WqZr8YtK', '/runs/run_4Fh8RmTq2LxW'].map(r => `/o/acme/projects/rumah/agents/yasaku-keuangan${r}`));
    expect(log.text()).toContain('approvalcard.approved|{"by":"Sari"} · approvalcard.via_whatsapp_reply|{"reply":"ya"}');
    expect(log.findAll('p.divider').map(p => p.text())).toHaveLength(2);
    expect(log.text()).toContain('approvalcard.expired');
    expect(log.findAll('section button').some(b => b.text().includes('approvalcard.approve_run'))).toBe(false);
    expect(w.get('aside').text()).toContain('Budi');
    expect(w.get('aside').getComponent(RouterLinkStub).props('to')).toBe('/o/acme/projects/rumah/end-users/eu_budi1');
    const th = useThreadsStore().byId('thr_grp3Kd9Q')!;
    const live = th.items;
    await button(w, 'thread.cancel_run').trigger('click');
    await flushPromises();
    const dialog = w.findAll('dialog').find(d => d.text().includes('thread.cancel_body'))!;
    expect((dialog.element as HTMLDialogElement).open).toBe(true);
    th.items = live.map(i => (i.kind === 'run' ? { ...i, run: { ...i.run, status: 'completed' as const } } : i));
    await flushPromises();
    expect((dialog.element as HTMLDialogElement).open).toBe(false);
    th.items = live;
    await flushPromises();
    await button(w, 'thread.cancel_run').trigger('click');
    await flushPromises();
    await dialog.findAll('button').find(b => b.text() === 'thread.cancel_run')!.trigger('click');
    await flushPromises();
    expect(useThreadsStore().byId('thr_grp3Kd9Q')!.status).toBe('cancelled');
    w.unmount();
  });

  it('shows who must answer an open approval and is a 404 for an unknown thread', async () => {
    seed('/agents/yasaku-keuangan/threads/thr_dm8Kq2Wx', { agent: 'yasaku-keuangan', thread: 'thr_dm8Kq2Wx' });
    const w = await mountPage(ThreadPage);
    expect(w.get('[role="log"]').text()).toContain('approvalcard.waiting_for|{"name":"Budi"}');
    expect(w.get('aside').text()).toContain('Simpan Rp40.000 dari Cash?');
    route.params = { ...route.params, thread: 'gone' };
    await flushPromises();
    expect(w.text()).toContain('error.thread_not_found');
    setActivePinia(createPinia());
    seed('/agents/yasaku-keuangan/threads/nope', { agent: 'yasaku-keuangan', thread: 'nope' });
    const errorHandler = vi.fn();
    await mountPage(ThreadPage, { global: { config: { errorHandler } } });
    expect(errorHandler.mock.calls[0]![0]).toMatchObject({ statusCode: 404 });
  });
});

describe('after a person is deleted', () => {
  it('shows their group messages as deleted and hides their name in lists and approvals', async () => {
    const people = useEndUsersStore();
    await people.load(projectId);
    people.remove('eu_sari2');
    seed('/agents/yasaku-keuangan/threads/thr_grp3Kd9Q', { agent: 'yasaku-keuangan', thread: 'thr_grp3Kd9Q' });
    const thread = await mountPage(ThreadPage);
    const log = thread.get('[role="log"]');
    expect(log.text()).toContain('chat.message_deleted');
    expect(log.text()).toContain('endusers.deleted_person');
    expect(log.text()).not.toContain('belanja pasar 215rb');
    expect(log.text()).toContain('approvalcard.approved|{"by":"endusers.deleted_person"}');
    expect(thread.get('aside').text()).toContain('endusers.deleted_person');
    seed('/agents/yasaku-keuangan/threads', { agent: 'yasaku-keuangan' });
    const list = await mountPage(ThreadsPage);
    expect(list.get('tbody tr').text()).toContain('endusers.deleted_person: chat.message_deleted');
    seed('/approvals', {}, ['agent:read', 'agent:write'], { tab: 'decided' });
    const inbox = await mountPage(ApprovalsPage);
    expect(inbox.get('tbody').text()).toContain('endusers.deleted_person · threads.kind_whatsapp_group');
    expect(inbox.get('tbody').text()).not.toContain('Sari');
  });
});

describe('approvals inbox', () => {
  it('opens a pending request as the shared card and approves it without a confirm', async () => {
    vi.useFakeTimers();
    seed('/approvals', {});
    const w = await mountPage(ApprovalsPage);
    const tabs = w.findAll('nav a');
    expect(tabs.map(a => a.text())).toEqual(['approvals.tab_pending 1', 'approvals.tab_decided', 'approvals.tab_expired']);
    expect(tabs[0]!.attributes('aria-current')).toBe('page');
    const card = w.get('details section');
    expect(card.findAll('dt').map(d => d.text())).toContain('Dompet');
    await card.findAll('button').find(b => b.text().includes('approvalcard.approve_run'))!.trigger('click');
    expect(w.text()).toContain('approvalcard.executing');
    await vi.runAllTimersAsync();
    await flushPromises();
    expect(w.text()).toContain('approvals.empty_pending');
    expect(useApprovalsStore().items[0]!.approval.status).toBe('approved');
  });

  it('says so in a toast and moves focus to the list after a decision', async () => {
    vi.useFakeTimers();
    seed('/approvals', {});
    const w = await mountPage(ApprovalsPage, { attachTo: document.body });
    const card = w.get('details section');
    await card.findAll('button').find(b => b.text().includes('approvalcard.approve_run'))!.trigger('click');
    await vi.advanceTimersByTimeAsync(1000);
    await flushPromises();
    expect(useToast().toasts.at(-1)).toMatchObject({ kind: 'success', text: 'approvals.approved_toast|{"title":"Simpan Rp40.000 dari Cash?"}' });
    expect(document.activeElement?.getAttribute('tabindex')).toBe('-1');
    expect(document.activeElement?.textContent).toContain('approvals.empty_pending');
    w.unmount();
  });

  it('shows a decision that came too late as expired', async () => {
    seed('/approvals', {});
    await useApprovalsStore().load(projectId);
    useNow().value = new Date(Date.now() - 60_000);
    useApprovalsStore().items[0]!.approval.expiresAt = new Date(Date.now() - 1000);
    const w = await mountPage(ApprovalsPage, { attachTo: document.body });
    await w.get('details section').findAll('button').find(b => b.text().includes('approvalcard.approve_run'))!.trigger('click');
    await flushPromises();
    expect(useToast().toasts.at(-1)).toMatchObject({ kind: 'error', text: 'approvals.expired_toast|{"title":"Simpan Rp40.000 dari Cash?"}' });
    expect(useApprovalsStore().expired.map(i => i.approval.id)).toContain('apr_kp1');
    expect(w.text()).toContain('approvals.empty_pending');
    w.unmount();
  });

  it('lists decided and expired requests with who decided, from the tab in the URL', async () => {
    seed('/approvals', {}, ['agent:read', 'agent:write'], { tab: 'decided' });
    const w = await mountPage(ApprovalsPage);
    const rows = w.findAll('tbody tr');
    expect(rows).toHaveLength(2);
    expect(rows[0]!.text()).toContain('approvalcard.approved|{"by":"Sari"} · approvalcard.via_whatsapp_reply|{"reply":"ya"}');
    expect(rows[1]!.text()).toContain('approvalcard.rejected|{"by":"Budi"} · approvalcard.via_console');
    setRoute({ path: '/o/acme/projects/rumah/approvals', params: { org: 'acme', project: 'rumah' }, query: { tab: 'expired' } });
    await flushPromises();
    expect(w.get('tbody').text()).toContain('approvalcard.expired');
  });

  it('falls back to Pending for an unknown tab and moves an expired request out of it', async () => {
    seed('/approvals', {}, ['agent:read', 'agent:write'], { tab: 'bogus' });
    useApprovalsStore().items = [];
    await useApprovalsStore().load(projectId);
    useApprovalsStore().items[0]!.approval.expiresAt = new Date(Date.now() - 1000);
    const w = await mountPage(ApprovalsPage);
    expect(w.findAll('nav a')[0]!.attributes('aria-current')).toBe('page');
    expect(w.text()).toContain('approvals.empty_pending');
    expect(w.findAll('nav a')[0]!.text()).toBe('approvals.tab_pending 0');
  });

  it('shows no decision buttons without agent:write', async () => {
    seed('/approvals', {}, ['agent:read']);
    const w = await mountPage(ApprovalsPage);
    expect(w.text()).toContain('approvals.cant_decide');
    expect(w.findAll('button').some(b => b.text().includes('approvalcard.approve_run'))).toBe(false);
  });
});
