import { create } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { RunSchema, RunStatus, ThreadMessageSchema, ThreadSchema } from '#gen/thread/v1/thread_pb';
import { connectError, setClient, setKnownKeys } from '#test/nuxt-globals';
import { useAgentsStore } from './agents';
import { pollIntervalMs, useThreadsStore } from './threads';

const agentId = 'Ag7pQ2wXk9LmT4vNc8RbZ';
const projectId = 'x6MH9WPWjktMwCBKHy4pF';
const threadId = 'Th3kP9wQ2mXr7LtV5nBcZ';
const runId = 'Rn2wQ8mX4vL9tB3nC7kPq';
const at = (min: number) => timestampFromDate(new Date(Date.UTC(2026, 10, 5, 5, min)));

const thread = (over = {}) => create(ThreadSchema, { id: threadId, projectId, agentId, kind: 'dm', endUserLabel: 'WhatsApp ·…5521', lastActivityAt: at(1), openRunStatus: RunStatus.RUNNING, runCount: 1n, ...over });
const run = (over = {}) => create(RunSchema, { id: runId, threadId, status: RunStatus.RUNNING, attempt: 1, agentVersion: 3, usage: { inputTokens: 0n, outputTokens: 0n }, startedAt: at(1), ...over });
const userMsg = create(ThreadMessageSchema, { seq: 1n, role: 'user', message: { role: 'user', content: 'Catat makan siang 25 ribu' }, status: 'consumed', runId, speakerLabel: 'WhatsApp ·…5521', createdAt: at(1) });
const callMsg = create(ThreadMessageSchema, { seq: 2n, role: 'assistant', message: { role: 'assistant', toolCalls: [{ id: 'call_1', type: 'function', function: { name: 'yasaku__record_expense', arguments: '{"amount":25000}' } }] }, status: 'consumed', runId, createdAt: at(1) });
const resultMsg = create(ThreadMessageSchema, { seq: 3n, role: 'tool', toolCallId: 'call_1', message: { role: 'tool', content: '{"saved":true}' }, status: 'consumed', runId, createdAt: at(1) });
const replyMsg = create(ThreadMessageSchema, { seq: 4n, role: 'assistant', message: { role: 'assistant', content: 'Tercatat.' }, status: 'consumed', runId, createdAt: at(2) });

function client(over: Record<string, unknown> = {}) {
  const c = {
    listThreads: vi.fn().mockResolvedValueOnce({ threads: [thread()], nextPageToken: 'p2' }).mockResolvedValue({ threads: [thread({ id: 'Th8vN4cR2kP7wQ9mXt3Lb', kind: 'api', endUserLabel: 'API user ·…a91f', openRunStatus: RunStatus.UNSPECIFIED })], nextPageToken: '' }),
    getThread: vi.fn().mockResolvedValue({ thread: thread({ endUserLabel: 'WhatsApp +62 812-3456-5521' }) }),
    listRuns: vi.fn().mockResolvedValue({ runs: [run()], nextPageToken: '' }),
    listThreadMessages: vi.fn().mockResolvedValueOnce({ messages: [userMsg, callMsg] }).mockResolvedValueOnce({ messages: [] }).mockResolvedValueOnce({ messages: [resultMsg, replyMsg] }).mockResolvedValue({ messages: [] }),
    getRun: vi.fn().mockResolvedValueOnce({ run: run() }).mockResolvedValue({ run: run({ status: RunStatus.COMPLETED, latencyMs: 4200n, costMicros: 782n, usage: { inputTokens: 1204n, outputTokens: 188n, estimated: true } }) }),
    cancelRun: vi.fn().mockResolvedValue({ run: run({ status: RunStatus.CANCELLED }) }),
    ...over,
  };
  setClient('$threadClient', c);
  return c;
}

describe('useThreadsStore on ThreadService', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    useAgentsStore().items = [{ id: agentId, projectId, slug: 'yasaku', name: 'Yasaku', description: '' }];
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('lists the agent\'s threads and pages with the cursor', async () => {
    const c = client();
    const store = useThreadsStore();
    await store.load(agentId, false);
    expect(c.listThreads).toHaveBeenCalledWith({ projectId, agentId, pageToken: '' });
    expect(store.threads.map(t => [t.kind, t.title, t.status])).toEqual([['whatsapp_dm', 'WhatsApp ·…5521', 'running']]);
    expect(store.cursor).toBe('p2');
    await store.loadMore(agentId);
    expect(c.listThreads).toHaveBeenLastCalledWith({ projectId, agentId, pageToken: 'p2' });
    expect(store.threads.map(t => [t.kind, t.status])).toEqual([['whatsapp_dm', 'running'], ['api', undefined]]);
    expect(store.cursor).toBe('');
    await store.loadMore(agentId);
    expect(c.listThreads).toHaveBeenCalledTimes(2);
  });

  it('never keeps a phone number in a thread or speaker label', async () => {
    client();
    const store = useThreadsStore();
    await store.load(agentId, false);
    await store.open(threadId);
    const th = store.byId(threadId)!;
    expect(th.title).toBe('WhatsApp …5521');
    const labels = [th.title, ...th.people.map(p => p.name), ...th.items.flatMap(i => (i.kind === 'user' ? [i.speaker ?? ''] : []))];
    expect(labels.filter(l => /\d{5,}/.test(l.replace(/\D/g, '')))).toEqual([]);
    store.stopPolling();
  });

  it('opens a thread, folds the tool result under its call, and polls every 2 s until the run settles', async () => {
    vi.useFakeTimers();
    const c = client();
    const store = useThreadsStore();
    await store.load(agentId, false);
    await store.open(threadId);
    expect(store.polling()).toBe(true);
    expect(store.byId(threadId)!.items.map(i => i.kind)).toEqual(['user', 'tool', 'run']);
    await vi.advanceTimersByTimeAsync(pollIntervalMs);
    expect(store.polling()).toBe(true);
    await vi.advanceTimersByTimeAsync(pollIntervalMs);
    expect(c.listThreadMessages).toHaveBeenLastCalledWith({ threadId, afterSeq: 2n, limit: 200 });
    const th = store.byId(threadId)!;
    expect(th.items.map(i => i.kind)).toEqual(['user', 'tool', 'agent', 'run']);
    expect(th.items[1]).toMatchObject({ name: 'yasaku__record_expense', args: { amount: 25000 }, result: '{"saved":true}', state: 'done' });
    expect(th.items[3]).toMatchObject({ run: { status: 'completed', seconds: 4.2, tokens: 1392, estimated: true, unpriced: false, costCents: 0.0782 } });
    expect(th.status).toBe('completed');
    expect(store.polling()).toBe(false);
    await vi.advanceTimersByTimeAsync(pollIntervalMs * 3);
    expect(c.getRun).toHaveBeenCalledTimes(2);
  });

  it('cancels a running run, shows its new status and stops polling', async () => {
    vi.useFakeTimers();
    const c = client();
    const store = useThreadsStore();
    await store.load(agentId, false);
    await store.open(threadId);
    await store.cancelRun(threadId, runId, 'Ann');
    expect(c.cancelRun).toHaveBeenCalledWith({ runId });
    expect(store.byId(threadId)!.status).toBe('cancelled');
    expect(store.byId(threadId)!.items.at(-1)).toMatchObject({ kind: 'run', run: { status: 'cancelled', unpriced: true } });
    expect(store.polling()).toBe(false);
  });

  it('stops polling once no run is running, and a cancel for an unloaded thread changes no thread', async () => {
    vi.useFakeTimers();
    const c = client();
    const store = useThreadsStore();
    await store.load(agentId, false);
    await store.open(threadId);
    expect(store.polling()).toBe(true);
    store.runs = [];
    await vi.advanceTimersByTimeAsync(pollIntervalMs);
    expect(store.polling()).toBe(false);
    expect(c.getRun).not.toHaveBeenCalled();
    const before = store.threads.map(t => t.id);
    await store.cancelRun('ThUnknown2345678923xyz', runId, 'Ann');
    expect(c.cancelRun).toHaveBeenCalledWith({ runId });
    expect(store.threads.map(t => t.id)).toEqual(before);
  });

  it('shows list and thread failures, keeps polling after a failed poll, and skips a missing thread', async () => {
    vi.useFakeTimers();
    setKnownKeys('apperror.GEN900');
    const c = client({ listThreads: vi.fn().mockRejectedValue(connectError('GEN900')), getRun: vi.fn().mockRejectedValue(new Error('offline')) });
    const store = useThreadsStore();
    await store.load(agentId, false);
    expect(store.error).toEqual({ message: 'apperror.GEN900', reference: 'error.ref|{"id":"req-1"}' });
    await store.open(threadId);
    await vi.advanceTimersByTimeAsync(pollIntervalMs);
    expect(store.error).toEqual({ message: 'error.load_thread' });
    expect(store.polling()).toBe(true);
    store.stopPolling();
    c.getThread.mockResolvedValueOnce({});
    await store.open('ThUnknown2345678923xyz');
    expect(store.byId('ThUnknown2345678923xyz')).toBeUndefined();
    c.getThread.mockRejectedValueOnce(new Error('offline'));
    await store.open(threadId);
    expect(store.error).toEqual({ message: 'error.load_thread' });
  });

  it('keeps the sample agent on samples, with no RPC, and a load-more failure shows', async () => {
    const c = client();
    const store = useThreadsStore();
    await store.load(agentId, true);
    await store.open('thr_grp3Kd9Q');
    expect(c.listThreads).not.toHaveBeenCalled();
    expect(c.getThread).not.toHaveBeenCalled();
    expect(store.threads.length).toBeGreaterThan(0);
    await store.load(agentId, false);
    c.listThreads.mockRejectedValue(new Error('offline'));
    await store.loadMore(agentId);
    expect(store.error).toEqual({ message: 'error.load_threads' });
  });
});
