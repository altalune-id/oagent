import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it } from 'vitest';
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
