import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { decisionStatus } from '~/components/run/runView';
import { useApprovalsStore } from './approvals';
import { useThreadsStore } from './threads';

describe('useThreadsStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('serves sample conversations only for the sample agent, once per agent', async () => {
    const store = useThreadsStore();
    await store.ensureLoaded('a1', false);
    expect([store.threads, store.runs]).toEqual([[], []]);
    await store.ensureLoaded('a2', true);
    expect(store.threads).toHaveLength(4);
    expect(store.runs).toHaveLength(8);
    store.threads = [];
    await store.ensureLoaded('a2', true);
    expect(store.threads).toEqual([]);
  });

  it('cancels a running run and stops its open tool calls', async () => {
    const store = useThreadsStore();
    await store.load('a2', true);
    store.cancelRun('thr_grp3Kd9Q', 'run_4Fh8RmTq2LxW', 'Budi');
    const thread = store.byId('thr_grp3Kd9Q')!;
    const runItem = thread.items.find(i => i.kind === 'run' && i.run.id === 'run_4Fh8RmTq2LxW')!;
    expect(runItem.kind === 'run' && runItem.run.status).toBe('cancelled');
    expect(store.runs.find(r => r.id === 'run_4Fh8RmTq2LxW')!.detail).toEqual({ key: 'runstatus.detail_cancelled', params: { name: 'Budi' } });
    const count = thread.items.length;
    store.cancelRun('thr_grp3Kd9Q', 'run_4Fh8RmTq2LxW', 'Budi');
    expect(thread.items).toHaveLength(count);
    expect(thread.status).toBe('cancelled');
    expect(thread.items.at(-1)).toMatchObject({ kind: 'notice', key: 'try.stopped' });
    expect(thread.items.filter(i => i.kind === 'tool' && i.state === 'running')).toEqual([]);
    expect(store.runs.find(r => r.id === 'run_4Fh8RmTq2LxW')!.status).toBe('cancelled');
    store.cancelRun('nope', 'nope', 'Budi');
    expect(store.byId('nope')).toBeUndefined();
  });
});

describe('useApprovalsStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('approves through a running state, rejects with a reason and ignores decided items', async () => {
    const store = useApprovalsStore();
    await store.ensureLoaded('p1');
    await store.ensureLoaded('p1');
    expect(store.pending.map(i => i.approval.id)).toEqual(['apr_kp1']);
    const run = store.decide('apr_kp1', true, 'Budi');
    expect(store.items[0]!.approval.status).toBe('executing');
    expect(store.pending).toHaveLength(1);
    await vi.runAllTimersAsync();
    await run;
    expect(store.items[0]!.approval).toMatchObject({ status: 'approved', decidedBy: 'Budi', decidedVia: { channel: 'console' } });
    expect(store.decided.map(i => i.approval.id)).toEqual(['apr_kp1', 'apr_ps2', 'apr_tr3']);
    expect(store.pending).toEqual([]);
    store.items[0]!.approval.status = 'pending';
    expect(await store.decide('apr_kp1', false, 'Budi', ' salah ')).toBe(true);
    expect(store.items[0]!.approval).toMatchObject({ status: 'rejected', reason: 'salah' });
    expect(await store.decide('apr_ps2', false, 'Budi')).toBe(false);
    expect(store.items[1]!.approval.status).toBe('approved');
    expect(await store.decide('nope', true, 'Budi')).toBe(false);
  });

  it('moves a pending request past its expiry to Expired and refuses to decide it', async () => {
    const store = useApprovalsStore();
    await store.load('p1');
    store.items[0]!.approval.expiresAt = new Date(Date.now() - 1000);
    expect(store.pending).toEqual([]);
    expect(store.expired.map(i => i.approval.id)).toEqual(['apr_kp1', 'apr_del1']);
    expect(await store.decide('apr_kp1', true, 'Budi')).toBe(false);
    expect(store.items[0]!.approval.status).toBe('pending');
  });
});

describe('decisionStatus', () => {
  it('shows a waiting run as awaiting approval and a decided one as completed', () => {
    expect(decisionStatus('waiting')).toBe('awaiting_approval');
    expect(decisionStatus('approved')).toBe('completed');
    expect(decisionStatus('rejected')).toBe('completed');
  });
});
