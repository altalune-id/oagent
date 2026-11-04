import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it } from 'vitest';
import { nextTick } from 'vue';
import { useApprovalsStore } from './approvals';
import { useEndUsersStore } from './endUsers';
import { useThreadsStore } from './threads';

describe('useEndUsersStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('loads once per project and keeps Try it testers out', async () => {
    const store = useEndUsersStore();
    await store.ensureLoaded('p1');
    expect(store.items.map(u => u.name)).toEqual(['Budi', 'Sari', 'cust_0001']);
    expect(store.items.flatMap(u => u.threads).some(th => th.kind === 'try')).toBe(false);
    store.items = [];
    await store.ensureLoaded('p1');
    expect(store.items).toEqual([]);
  });

  it('forgets one memory and deletes a person, ignoring unknown ids', async () => {
    const store = useEndUsersStore();
    await store.load('p1');
    store.forgetMemory('eu_sari2', 'mem_7Lp2WkR9');
    store.forgetMemory('nope', 'mem_9Dz4HbM6');
    expect(store.byId('eu_sari2')!.memories.map(m => m.id)).toEqual(['mem_9Dz4HbM6']);
    store.remove('eu_sari2');
    expect(store.items.map(u => u.id)).toEqual(['eu_budi1', 'eu_api3']);
  });

  it('removes the person from conversations and approvals already loaded, and from ones loaded later', async () => {
    const store = useEndUsersStore();
    await store.load('p1');
    const threads = useThreadsStore();
    await threads.load('a1', true);
    store.remove('eu_budi1');
    store.remove('nope');
    await nextTick();
    expect(threads.byId('thr_dm8Kq2Wx')).toBeUndefined();
    expect(threads.byId('thr_grp3Kd9Q')!.people[0]).toEqual({ name: '', deleted: true });
    const approvals = useApprovalsStore();
    await approvals.load('p1');
    expect(approvals.items.some(i => i.approval.id === 'apr_kp1')).toBe(false);
    await store.load('p1');
    expect(store.items.map(u => u.id)).toEqual(['eu_sari2', 'eu_api3']);
  });

  it('renames them in approvals that were loaded before the delete', async () => {
    const store = useEndUsersStore();
    await store.load('p1');
    const approvals = useApprovalsStore();
    await approvals.load('p1');
    store.remove('eu_sari2');
    await nextTick();
    expect(approvals.items.find(i => i.approval.id === 'apr_ps2')!.approval).toMatchObject({ decidedBy: '', deciderDeleted: true });
  });
});
