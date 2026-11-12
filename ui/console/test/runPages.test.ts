// @vitest-environment happy-dom
import { create } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { flushPromises, RouterLinkStub } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { WhoamiResponseSchema } from '#gen/auth/v1/auth_pb';
import { RunSchema, RunStatus, ThreadMessageSchema, ThreadSchema } from '#gen/thread/v1/thread_pb';
import { mountPage } from '#test/mount-page';
import { setClient, setKnownKeys, setRoute } from '#test/nuxt-globals';
import RunPage from '~/pages/o/[org]/projects/[project]/agents/[agent]/runs/[run].vue';
import RunsPage from '~/pages/o/[org]/projects/[project]/agents/[agent]/runs/index.vue';
import { useOrgsStore } from '~/stores/orgs';
import { useProjectsStore } from '~/stores/projects';

const orgId = 'B6rTn1YkM8wQz3XpL5vGd';
const projectId = 'x6MH9WPWjktMwCBKHy4pF';
const yasaku = { id: 'Y4sKu8RmT2pLq6WnX9cVb', projectId, slug: 'yasaku-keuangan', name: 'Yasaku Keuangan', description: '', publishedVersion: 4 };

beforeEach(() => {
  setActivePinia(createPinia());
  useOrgsStore().items = [{ id: orgId, slug: 'acme', name: 'Acme', role: 'owner', permissions: [] }];
  useProjectsStore().items = [{ id: projectId, orgId, slug: 'rumah', name: 'Rumah Tangga' }];
  setClient('$authClient', { whoami: vi.fn().mockResolvedValue(create(WhoamiResponseSchema, { name: 'Budi', permissions: ['agent:read', 'agent:write'] })) });
  setClient('$agentClient', { listAgents: vi.fn().mockResolvedValue({ agents: [yasaku] }) });
  setRoute({ path: '/o/acme/projects/rumah/agents/yasaku-keuangan/runs', params: { org: 'acme', project: 'rumah', agent: 'yasaku-keuangan' } });
});

describe('runs page', () => {
  it('lists one sample run per status with its conversation, detail and usage, filtered by status', async () => {
    const w = await mountPage(RunsPage);
    expect(w.get('.badge').text()).toBe('preview.badge');
    expect(w.get('[role="note"]').text()).toBe('run.preview_note');
    const rows = w.findAll('tbody tr');
    expect(rows).toHaveLength(8);
    expect(rows[1]!.text()).toContain('runstatus.awaiting_approval');
    expect(rows[1]!.text()).toContain('runstatus.detail_awaiting|{"name":"Budi"}');
    expect(rows[1]!.find('a[href]').exists() || rows[1]!.findAll('a').length).toBeTruthy();
    await w.get('select').setValue('failed');
    expect(w.findAll('tbody tr')).toHaveLength(1);
    expect(w.get('tbody').text()).toContain('cust_0001');
  });

  it('lists a real agent\'s runs from ListRuns by agent, newest first, with no preview', async () => {
    const tagihan = { id: 'Tg8hN3kP2wQ7mXr4VcB9z', projectId, slug: 'tagihan', name: 'Tagihan', description: '', publishedVersion: 2 };
    setClient('$agentClient', { listAgents: vi.fn().mockResolvedValue({ agents: [tagihan] }), getAgentConfig: vi.fn().mockResolvedValue({}) });
    setRoute({ path: '/o/acme/projects/rumah/agents/tagihan/runs', params: { org: 'acme', project: 'rumah', agent: 'tagihan' } });
    const threadId = 'Th7sK2pQ9mXr4StV8nBcW';
    const at = (min: number) => timestampFromDate(new Date(Date.UTC(2026, 10, 5, 5, min)));
    const c = {
      listThreads: vi.fn().mockResolvedValue({ threads: [create(ThreadSchema, { id: threadId, projectId, agentId: tagihan.id, kind: 'dm', endUserLabel: 'Sari', runCount: 2n })], nextPageToken: '' }),
      listRuns: vi.fn().mockResolvedValue({ runs: [
        create(RunSchema, { id: 'Rn2wQ8mX4vZ9tB3nC7kPq', threadId, status: RunStatus.COMPLETED, agentVersion: 2, usage: { inputTokens: 800n, outputTokens: 76n }, latencyMs: 2400n, costMicros: 1200n, startedAt: at(9) }),
        create(RunSchema, { id: 'Rn5tH2kW8pQ3mXv7ZcB4n', threadId: 'Th9zZ9zZ9zZ9zZ9zZ9zZ9', status: RunStatus.FAILED, errorCode: 'RUN004', agentVersion: 0, agentVersionKind: 'draft', usage: {}, startedAt: at(3) }),
      ], nextPageToken: '' }),
    };
    setClient('$threadClient', c);
    const w = await mountPage(RunsPage);
    await flushPromises();
    expect(c.listRuns).toHaveBeenCalledWith({ projectId, agentId: tagihan.id, pageSize: 200 });
    expect(w.find('[role="note"]').exists()).toBe(false);
    expect(w.get('caption').text()).toBe('run.caption');
    const rows = w.findAll('tbody tr');
    expect(rows.map(r => r.text().includes('Sari'))).toEqual([true, false]);
    expect(rows[1]!.text()).toContain('runs.thread_untitled');
    expect(rows[1]!.text()).toContain('runstatus.detail_error|{"code":"RUN004"}');
    expect(rows[1]!.text()).toContain('thread.unpriced');
    const priced = rows[0]!.get('[title]');
    expect([priced.text(), priced.attributes('title')]).toEqual(['< $0.01', '$0.0012']);
    expect(rows.map(r => r.findAll('.badge').map(b => b.text()))).toEqual([[], ['run.draft']]);
    const links = w.findAllComponents(RouterLinkStub).map(l => l.props('to'));
    expect(links).toEqual(expect.arrayContaining(['/o/acme/projects/rumah/agents/tagihan/runs/Rn2wQ8mX4vZ9tB3nC7kPq', `/o/acme/projects/rumah/agents/tagihan/threads/${threadId}`]));
  });

  it('shows a runs failure on its own, keeps the conversations loaded, and retries', async () => {
    setKnownKeys('error.load_runs');
    const tagihan = { id: 'Tg8hN3kP2wQ7mXr4VcB9z', projectId, slug: 'tagihan', name: 'Tagihan', description: '', publishedVersion: 2 };
    setClient('$agentClient', { listAgents: vi.fn().mockResolvedValue({ agents: [tagihan] }), getAgentConfig: vi.fn().mockResolvedValue({}) });
    setRoute({ path: '/o/acme/projects/rumah/agents/tagihan/runs', params: { org: 'acme', project: 'rumah', agent: 'tagihan' } });
    const c = {
      listThreads: vi.fn().mockResolvedValue({ threads: [create(ThreadSchema, { id: 'Th7sK2pQ9mXr4StV8nBcW', projectId, agentId: tagihan.id, kind: 'dm', endUserLabel: 'Sari' })], nextPageToken: '' }),
      listRuns: vi.fn().mockRejectedValueOnce(new Error('offline')).mockResolvedValue({ runs: [], nextPageToken: '' }),
    };
    setClient('$threadClient', c);
    const w = await mountPage(RunsPage);
    await flushPromises();
    expect(w.get('[role="alert"]').text()).toContain('error.load_runs');
    const { useThreadsStore } = await import('~/stores/threads');
    expect([useThreadsStore().error, useThreadsStore().threads.length]).toEqual([null, 1]);
    await w.findAll('button').find(b => b.text() === 'common.retry')!.trigger('click');
    await flushPromises();
    expect(c.listThreads).toHaveBeenCalledTimes(1);
    expect(c.listRuns).toHaveBeenCalledTimes(2);
    expect(w.find('[role="alert"]').exists()).toBe(false);
    expect(w.text()).toContain('runs.empty');
  });

  it('shows the empty state for a status without runs', async () => {
    const w = await mountPage(RunsPage);
    const { useThreadsStore } = await import('~/stores/threads');
    useThreadsStore().runs = [];
    await flushPromises();
    expect(w.text()).toContain('runs.empty');
  });
});

describe('run viewer', () => {
  const runId = 'Rn2wQ8mX4vZ9tB3nC7kPq';
  const threadId = 'Th7sK2pQ9mXr4StV8nBcW';
  const at = (min: number) => timestampFromDate(new Date(Date.UTC(2026, 10, 5, 5, min)));
  const said = (seq: bigint, role: string, content: string, ran = true) => create(ThreadMessageSchema, { seq, role, status: 'consumed', runId: ran ? runId : '', speakerLabel: role === 'user' ? 'WhatsApp …90' : '', message: { role, content }, createdAt: at(Number(seq)) });

  function threadClient(over: Record<string, unknown> = {}) {
    const c = {
      getRun: vi.fn().mockResolvedValue({ run: create(RunSchema, { id: runId, threadId, status: RunStatus.COMPLETED, attempt: 1, agentVersion: 4, usage: { inputTokens: 812n, outputTokens: 64n }, latencyMs: 2400n, startedAt: at(3), endedAt: at(4) }) }),
      getThread: vi.fn().mockResolvedValue({ thread: create(ThreadSchema, { id: threadId, projectId, agentId: yasaku.id, kind: 'dm', endUserLabel: 'WhatsApp …90', runCount: 1n }) }),
      listThreadMessages: vi.fn().mockResolvedValue({ messages: [said(1n, 'user', 'kemarin', false), said(3n, 'user', 'Berapa pengeluaran minggu ini?'), said(4n, 'assistant', 'Rp420.000')] }),
      ...over,
    };
    setClient('$threadClient', c);
    return c;
  }

  beforeEach(() => {
    vi.stubGlobal('definePageMeta', vi.fn());
    setRoute({ path: `/o/acme/projects/rumah/agents/yasaku-keuangan/runs/${runId}`, params: { org: 'acme', project: 'rumah', agent: 'yasaku-keuangan', run: runId } });
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('shows the run\'s facts, links to its conversation and lists only the messages it took or wrote, with no preview', async () => {
    const c = threadClient();
    const w = await mountPage(RunPage);
    await flushPromises();
    expect(c.getRun).toHaveBeenCalledWith({ runId });
    expect(w.get('h1').text()).toContain(runId);
    expect(w.text()).toContain('run.started_live|{"version":4');
    expect(w.findAll('header dl dd').map(d => d.text())).toEqual(['runstatus.completed', '2.4s', '876', 'thread.unpriced']);
    const link = w.findAllComponents(RouterLinkStub).find(l => l.text() === 'run.open_thread')!;
    expect(link.props('to')).toBe(`/o/acme/projects/rumah/agents/yasaku-keuangan/threads/${threadId}`);
    expect(w.findAll('.chat-bubble').map(b => b.text())).toEqual(['Berapa pengeluaran minggu ini?', 'Rp420.000']);
    expect(w.text()).not.toContain('preview');
    expect(w.find('.badge-info').exists()).toBe(false);
    expect(w.text()).not.toContain('run.attempt');
  });

  it('shows embedding tokens and knowledge searches next to the tokens, only when the run used them', async () => {
    const c = threadClient();
    c.getRun.mockResolvedValue({ run: create(RunSchema, { id: runId, threadId, status: RunStatus.COMPLETED, attempt: 1, agentVersion: 4, usage: { inputTokens: 812n, outputTokens: 64n, embeddingTokens: 12n }, knowledgeRetrieveCalls: 1, knowledgeRetrieveMs: 340n, latencyMs: 2400n, startedAt: at(3), endedAt: at(4) }) });
    const w = await mountPage(RunPage);
    await flushPromises();
    expect(w.findAll('header dl dt').map(d => d.text())).toEqual(['run.status', 'run.duration', 'run.tokens', 'run.embedding_tokens', 'run.knowledge_search', 'run.cost']);
    expect(w.findAll('header dl dd').map(d => d.text())).toEqual(['runstatus.completed', '2.4s', '876', '12', 'run.knowledge_calls|{"n":1,"ms":"340"}', 'thread.unpriced']);
  });

  it('says a run on the unpublished draft ran the draft, not version 0', async () => {
    threadClient({ getRun: vi.fn().mockResolvedValue({ run: create(RunSchema, { id: runId, threadId, status: RunStatus.COMPLETED, attempt: 1, agentVersion: 0, agentVersionKind: 'draft', usage: {}, startedAt: at(3) }) }) });
    const w = await mountPage(RunPage);
    await flushPromises();
    expect(w.text()).toContain('run.started_draft|');
    expect(w.text()).not.toContain('run.started_live');
  });

  it('names a retried run\'s attempt and an empty run', async () => {
    threadClient({
      getRun: vi.fn().mockResolvedValue({ run: create(RunSchema, { id: runId, threadId, status: RunStatus.FAILED, attempt: 2, agentVersion: 4, errorCode: 'RUN004', usage: { estimated: true }, costMicros: 0n, startedAt: at(3) }) }),
      listThreadMessages: vi.fn().mockResolvedValue({ messages: [] }),
    });
    const w = await mountPage(RunPage);
    await flushPromises();
    expect(w.text()).toContain('run.attempt|{"n":2}');
    expect(w.text()).toContain('runstatus.detail_error|{"code":"RUN004"}');
    expect(w.text()).toContain('thread.tokens_estimated');
    expect(w.text()).toContain('run.no_messages');
  });

  it('shows a retryable error, and a missing run as not found with a way back', async () => {
    setKnownKeys('error.load_run');
    const c = threadClient({ getRun: vi.fn().mockRejectedValueOnce(new Error('offline')).mockResolvedValue({}) });
    const w = await mountPage(RunPage);
    await flushPromises();
    expect(w.get('[role="alert"]').text()).toContain('error.load_run');
    expect(w.get('h1').classes()).toContain('sr-only');
    await w.findAll('button').find(b => b.text() === 'common.retry')!.trigger('click');
    await flushPromises();
    expect(c.getRun).toHaveBeenCalledTimes(2);
    expect(w.get('h1').text()).toBe('error.run_not_found');
    expect(w.findAllComponents(RouterLinkStub).find(l => l.text() === 'run.all')!.props('to')).toBe('/o/acme/projects/rumah/agents/yasaku-keuangan/runs');
  });
});
