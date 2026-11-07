import { create } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { EndUserSchema, EndUserThreadSchema, IdentityKind } from '#gen/enduser/v1/enduser_pb';
import { connectError, deferred, setClient, setKnownKeys } from '#test/nuxt-globals';
import { useEndUsersStore } from './endUsers';

const projectId = 'x6MH9WPWjktMwCBKHy4pF';
const at = (min: number) => timestampFromDate(new Date(Date.UTC(2026, 10, 5, 5, min)));
const sari = create(EndUserSchema, { id: 'Eu5sR2kP8mQx4StV7nBcW', projectId, displayName: 'Sari', identityKind: IdentityKind.WA_PHONE, identityHint: '90', firstSeenAt: at(1), lastSeenAt: at(9), threadCount: 2n });
const anon = create(EndUserSchema, { id: 'Eu7tH3kQ9wXr2MnV5bCzD', projectId, identityKind: IdentityKind.WIDGET_ANON, threadCount: 1n });

describe('useEndUsersStore on EndUserService', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('lists a project\'s end users once, with the name search trimmed and sent to the server', async () => {
    const c = { listEndUsers: vi.fn().mockResolvedValue({ endUsers: [sari, anon], nextPageToken: '' }) };
    setClient('$endUserClient', c);
    const store = useEndUsersStore();
    await store.ensureLoaded(projectId);
    await store.ensureLoaded(projectId);
    expect(c.listEndUsers).toHaveBeenCalledTimes(1);
    expect(c.listEndUsers).toHaveBeenCalledWith({ projectId, query: '', pageSize: 200 });
    expect(store.items[0]).toEqual({ id: sari.id, displayName: 'Sari', identityKind: IdentityKind.WA_PHONE, identityHint: '90', firstSeenAt: new Date(Date.UTC(2026, 10, 5, 5, 1)), lastSeenAt: new Date(Date.UTC(2026, 10, 5, 5, 9)), threadCount: 2 });
    expect(store.items[1]).toMatchObject({ displayName: '', firstSeenAt: undefined, threadCount: 1 });
    await store.load(projectId, '  sar ');
    expect(c.listEndUsers).toHaveBeenLastCalledWith({ projectId, query: 'sar', pageSize: 200 });
  });

  it('keeps only the newest search when answers arrive out of order, and shows a failure', async () => {
    setKnownKeys('error.load_end_users');
    const slow = deferred<{ endUsers: typeof sari[], nextPageToken: string }>();
    const c = { listEndUsers: vi.fn().mockReturnValueOnce(slow.promise).mockResolvedValueOnce({ endUsers: [anon], nextPageToken: '' }).mockRejectedValueOnce(new Error('offline')) };
    setClient('$endUserClient', c);
    const store = useEndUsersStore();
    const first = store.load(projectId, 's');
    await store.load(projectId, 'web');
    slow.resolve({ endUsers: [sari], nextPageToken: '' });
    await first;
    expect([store.items.map(u => u.id), store.loading]).toEqual([[anon.id], false]);
    await store.load(projectId);
    expect(store.error?.message).toBe('error.load_end_users');
  });

  it('loads one person with their threads; a missing one is none, other failures show', async () => {
    setKnownKeys('error.load_end_users');
    const thread = create(EndUserThreadSchema, { id: 'Th7sK2pQ9mXr4StV8nBcW', agentId: 'Ag7pQ2wXk9MmT4vNc8RbZ', kind: 'dm', lastActivityAt: at(9) });
    const c = { getEndUser: vi.fn().mockResolvedValueOnce({ endUser: sari, threads: [thread] }).mockRejectedValueOnce(connectError('EUS001')).mockRejectedValueOnce(new Error('offline')) };
    setClient('$endUserClient', c);
    const store = useEndUsersStore();
    await store.loadOne(sari.id);
    expect(store.detail).toMatchObject({ user: { id: sari.id, displayName: 'Sari' }, threads: [{ id: thread.id, agentId: thread.agentId, kind: 'dm', lastActivityAt: new Date(Date.UTC(2026, 10, 5, 5, 9)) }] });
    await store.loadOne('Eu9zZ9zZ9zZ9zZ9zZ9zZ9');
    expect([store.detail, store.detailError]).toEqual([null, null]);
    await store.loadOne(sari.id);
    expect([store.detail, store.detailError?.message, store.detailLoading]).toEqual([null, 'error.load_end_users', false]);
  });
});
