import { create } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { MemorySchema } from '#gen/memory/v1/memory_pb';
import { connectError, deferred, setClient, setKnownKeys } from '#test/nuxt-globals';
import { useMemoriesStore } from './memories';

const person = 'Eu4pKq7WmN2xRt8VbZc3H';
const support = 'Ag2bVn7QxK4pRt9KwZc3H';
const storeId = 'Ms4tR7wQ2xKp9VbNc3HzL';
const at = new Date('2026-11-10T08:00:00Z');
const fact = (id: string, text: string, agentId = support, sourceRunId = 'Rn2wQ8mX4vZ9tB3nC7kPq') => create(MemorySchema, { id, text, agentId, createdAt: timestampFromDate(at), sourceRunId });

function client(over: Record<string, ReturnType<typeof vi.fn>> = {}) {
  const c = {
    listMemories: vi.fn().mockResolvedValue({ memories: [fact('f1', 'Prefers Indonesian')], nextPageToken: 'p2', total: 2 }),
    deleteMemory: vi.fn().mockResolvedValue({}),
    clearMemories: vi.fn().mockResolvedValue({ deleted: 2 }),
    ...over,
  };
  setClient('$memoryClient', c);
  return c;
}

describe('useMemoriesStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('loads a person\'s memories 50 at a time, with the agent filter, and more on request', async () => {
    const c = client();
    const store = useMemoriesStore();
    await store.load(storeId, person);
    expect(c.listMemories).toHaveBeenLastCalledWith({ storeId, endUserId: person, pageSize: 50 });
    expect(store.items).toEqual([{ id: 'f1', text: 'Prefers Indonesian', agentId: support, createdAt: at, sourceRunId: 'Rn2wQ8mX4vZ9tB3nC7kPq' }]);
    expect([store.total, store.nextPageToken]).toEqual([2, 'p2']);
    c.listMemories.mockResolvedValueOnce({ memories: [fact('f2', 'Card ends in 4421', support, '')], nextPageToken: '', total: 2 });
    await store.loadMore();
    expect(c.listMemories).toHaveBeenLastCalledWith({ storeId, endUserId: person, pageSize: 50, pageToken: 'p2' });
    expect(store.items.map(m => [m.id, m.sourceRunId])).toEqual([['f1', 'Rn2wQ8mX4vZ9tB3nC7kPq'], ['f2', '']]);
    expect(store.nextPageToken).toBe('');
    await store.loadMore();
    expect(c.listMemories).toHaveBeenCalledTimes(2);
    await store.load(storeId, person, support);
    expect(c.listMemories).toHaveBeenLastCalledWith({ storeId, endUserId: person, agentId: support, pageSize: 50 });
    expect([store.storeId, store.agentId]).toEqual([storeId, support]);
  });

  it('drops an older answer when the filter changes mid-load', async () => {
    const slow = deferred<unknown>();
    const c = client();
    c.listMemories.mockReturnValueOnce(slow.promise);
    const store = useMemoriesStore();
    const first = store.load(storeId, person);
    await store.load(storeId, person, support);
    slow.resolve({ memories: [fact('old', 'Stale')], nextPageToken: '', total: 1 });
    await first;
    expect(store.items.map(m => m.id)).toEqual(['f1']);
  });

  it('ignores a failure of an older load, and keeps a memory without a saved date', async () => {
    const slow = deferred<unknown>();
    const c = client();
    c.listMemories.mockReturnValueOnce(slow.promise).mockResolvedValueOnce({ memories: [create(MemorySchema, { id: 'f3', text: 'No date', agentId: support })], nextPageToken: '', total: 1 });
    const store = useMemoriesStore();
    const first = store.load(storeId, person);
    await store.load(storeId, person, support);
    slow.reject(new Error('fetch failed'));
    await first;
    expect(store.error).toBeNull();
    expect(store.items).toEqual([{ id: 'f3', text: 'No date', agentId: support, createdAt: undefined, sourceRunId: '' }]);
  });

  it('deletes one memory in place and counts one less, always naming the person', async () => {
    const c = client();
    const store = useMemoriesStore();
    await store.load(storeId, person);
    expect(await store.remove('f1')).toBe('deleted');
    expect(c.deleteMemory).toHaveBeenCalledWith({ storeId, endUserId: person, id: 'f1' });
    expect(store.items).toEqual([]);
    expect(store.total).toBe(1);
  });

  it('treats a not-found delete as already gone and reloads the list', async () => {
    const c = client({ deleteMemory: vi.fn().mockRejectedValue(connectError('MEM001')) });
    const store = useMemoriesStore();
    await store.load(storeId, person, support);
    c.listMemories.mockResolvedValueOnce({ memories: [], nextPageToken: '', total: 0 });
    expect(await store.remove('f1')).toBe('gone');
    expect(store.actionError).toBeNull();
    expect(c.listMemories).toHaveBeenLastCalledWith({ storeId, endUserId: person, agentId: support, pageSize: 50 });
    expect([store.items, store.total]).toEqual([[], 0]);
  });

  it('clears with the current agent filter, or every agent without one, and returns the count cleared', async () => {
    const c = client();
    const store = useMemoriesStore();
    await store.load(storeId, person, support);
    expect(await store.clear()).toBe(2);
    expect(c.clearMemories).toHaveBeenLastCalledWith({ storeId, endUserId: person, agentId: support });
    expect([store.items, store.total, store.nextPageToken]).toEqual([[], 0, '']);
    await store.load(storeId, person);
    await store.clear();
    expect(c.clearMemories).toHaveBeenLastCalledWith({ storeId, endUserId: person });
  });

  it('keeps the rows and shows the error when a delete or clear fails', async () => {
    setKnownKeys('error.delete_memory', 'error.clear_memories');
    const c = client({ deleteMemory: vi.fn().mockRejectedValue(new Error('fetch failed')), clearMemories: vi.fn().mockRejectedValue(new Error('fetch failed')) });
    const store = useMemoriesStore();
    await store.load(storeId, person);
    expect(await store.remove('f1')).toBe(false);
    expect(store.items.map(m => m.id)).toEqual(['f1']);
    expect(store.actionError?.message).toBe('error.delete_memory');
    expect(await store.clear()).toBeNull();
    expect(store.items.map(m => m.id)).toEqual(['f1']);
    expect(store.actionError?.message).toBe('error.clear_memories');
    expect(c.listMemories).toHaveBeenCalledOnce();
  });

  it('shows a load failure with its message', async () => {
    setKnownKeys('error.load_memories');
    client({ listMemories: vi.fn().mockRejectedValue(new Error('fetch failed')) });
    const store = useMemoriesStore();
    await store.load(storeId, person);
    expect(store.error?.message).toBe('error.load_memories');
    expect(store.loading).toBe(false);
  });
});
