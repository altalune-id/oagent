import { create } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { MemoryBackend, MemoryStoreSchema } from '#gen/memory/v1/memory_pb';
import { deferred, setClient, setKnownKeys } from '#test/nuxt-globals';
import { useMemoryStoresStore } from './memoryStores';

const projectId = 'x6MH9WPWjktMwCBKHy4pF';
const at = new Date('2026-11-12T08:00:00Z');
const pb = (id: string, name: string, usedBy: { agentId: string, agentName: string }[] = []) => create(MemoryStoreSchema, { id, projectId, name, backend: MemoryBackend.NATIVE, usedBy, createdAt: timestampFromDate(at) });

describe('useMemoryStoresStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('loads a project\'s stores with the agents using them and whether any backend is on, once per project', async () => {
    const listMemoryStores = vi.fn().mockResolvedValue({ stores: [pb('Ms4tR7wQ2xKp9VbNc3HzL', 'People', [{ agentId: 'Ag7pQ2wXk9MmT4vNc8RbZ', agentName: 'Support' }])], enabledBackends: [MemoryBackend.NATIVE] });
    setClient('$memoryClient', { listMemoryStores });
    const store = useMemoryStoresStore();
    await store.ensureLoaded(projectId);
    await store.ensureLoaded(projectId);
    expect(listMemoryStores).toHaveBeenCalledOnce();
    expect(listMemoryStores).toHaveBeenCalledWith({ projectId });
    expect(store.items).toEqual([{ id: 'Ms4tR7wQ2xKp9VbNc3HzL', name: 'People', usedBy: [{ id: 'Ag7pQ2wXk9MmT4vNc8RbZ', name: 'Support' }], createdAt: at, rememberApiUsers: false }]);
    expect(store.on).toBe(true);
  });

  it('is off with no enabled backend, and keeps a store without a date', async () => {
    setClient('$memoryClient', { listMemoryStores: vi.fn().mockResolvedValue({ stores: [create(MemoryStoreSchema, { id: 's', name: 'Old' })], enabledBackends: [] }) });
    const store = useMemoryStoresStore();
    await store.load(projectId);
    expect(store.on).toBe(false);
    expect(store.items[0]!.createdAt).toBeUndefined();
  });

  it('shows a load failure, and drops an older answer', async () => {
    setKnownKeys('error.load_memory_stores');
    const slow = deferred<unknown>();
    const listMemoryStores = vi.fn().mockReturnValueOnce(slow.promise).mockRejectedValueOnce(new Error('offline'));
    setClient('$memoryClient', { listMemoryStores });
    const store = useMemoryStoresStore();
    const first = store.load(projectId);
    await store.load(projectId);
    slow.resolve({ stores: [pb('s', 'Stale')], enabledBackends: [MemoryBackend.NATIVE] });
    await first;
    expect(store.items).toEqual([]);
    expect(store.error?.message).toBe('error.load_memory_stores');
    expect(store.loading).toBe(false);
  });

  it('ignores a failure of an older load', async () => {
    const slow = deferred<unknown>();
    setClient('$memoryClient', { listMemoryStores: vi.fn().mockReturnValueOnce(slow.promise).mockResolvedValueOnce({ stores: [pb('a', 'A')], enabledBackends: [MemoryBackend.NATIVE] }) });
    const store = useMemoryStoresStore();
    const first = store.load(projectId);
    await store.load(projectId);
    slow.reject(new Error('offline'));
    await first;
    expect([store.error, store.items.map(s => s.name)]).toEqual([null, ['A']]);
  });

  it('adds a created store and removes a deleted one', async () => {
    const deleteMemoryStore = vi.fn().mockResolvedValue({});
    setClient('$memoryClient', { listMemoryStores: vi.fn().mockResolvedValue({ stores: [pb('a', 'A')], enabledBackends: [MemoryBackend.NATIVE] }), deleteMemoryStore });
    const store = useMemoryStoresStore();
    await store.load(projectId);
    store.add(pb('b', 'B'));
    expect(store.items.map(s => s.name)).toEqual(['A', 'B']);
    await store.remove('a');
    expect(deleteMemoryStore).toHaveBeenCalledWith({ id: 'a' });
    expect(store.items.map(s => s.id)).toEqual(['b']);
  });
});
