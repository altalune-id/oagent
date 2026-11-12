import type { MessageInitShape } from '@bufbuild/protobuf';
import { create } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ApprovalSchema } from '#gen/approval/v1/approval_pb';
import { connectError, setClient } from '#test/nuxt-globals';
import { useAgentsStore } from './agents';
import { useApprovalsStore } from './approvals';

const projectId = 'x6MH9WPWjktMwCBKHy4pF';
const soon = timestampFromDate(new Date(Date.now() + 3_600_000));
const pb = (id: string, over: MessageInitShape<typeof ApprovalSchema> = {}) => create(ApprovalSchema, { id, projectId, agentId: 'Ag1', threadId: 'Th1', runId: 'Rn1', status: 'pending', channel: 'whatsapp', tools: ['yasaku__record_expense'], preview: 'yasaku · record_expense\namount: 1', expiresAt: soon, ...over });

function client(over: Record<string, unknown> = {}) {
  const c = {
    listApprovals: vi.fn(async ({ status, pageToken }: { status: string, pageToken: string }) => {
      if (status === 'pending') {
        return { approvals: pageToken ? [pb('Ap3')] : [pb('Ap1'), pb('Ap2')], nextPageToken: pageToken ? '' : 'p2', pendingCount: 3n };
      }
      if (status === 'expired') {
        return { approvals: [pb('Ex1', { status: 'expired' })], nextPageToken: '', pendingCount: 3n };
      }
      return { approvals: [pb('Ok1', { status: 'approved' }), pb('No1', { status: 'rejected' })], nextPageToken: '', pendingCount: 3n };
    }),
    getApproval: vi.fn(async ({ id }: { id: string }) => ({ approval: pb(id, { status: 'approved', decidedByLabel: 'Sari', decidedVia: 'whatsapp' }) })),
    decideApproval: vi.fn(async ({ id, approved, text }: { id: string, approved: boolean, text: string }) => ({ approval: pb(id, { status: approved ? 'approved' : 'rejected', decisionText: text, decidedByLabel: 'Budi', decidedVia: 'console' }) })),
    ...over,
  };
  setClient('$approvalClient', c);
  return c;
}

beforeEach(() => {
  setActivePinia(createPinia());
  useAgentsStore().items = [{ id: 'Ag1', projectId, slug: 'yasaku', name: 'Yasaku' } as never];
});

describe('approvals store', () => {
  it('lists each tab from the server: pending and expired by status, decided as approved and rejected only, with Load more', async () => {
    const c = client();
    const store = useApprovalsStore();
    await store.ensureLoaded(projectId, 'pending');
    await store.ensureLoaded(projectId, 'pending');
    expect(c.listApprovals).toHaveBeenCalledTimes(1);
    expect(c.listApprovals).toHaveBeenCalledWith({ projectId, status: 'pending', pageToken: '', pageSize: 50 });
    expect([store.pending.map(i => i.approval.id), store.pendingCount, store.more.pending]).toEqual([['Ap1', 'Ap2'], 3, true]);
    expect(store.pending[0]).toMatchObject({ agentName: 'Yasaku', agentSlug: 'yasaku' });
    await store.loadMore('pending');
    expect([store.pending.map(i => i.approval.id), store.more.pending]).toEqual([['Ap1', 'Ap2', 'Ap3'], false]);
    await store.ensureLoaded(projectId, 'decided');
    expect(c.listApprovals).toHaveBeenLastCalledWith({ projectId, status: 'decided', pageToken: '', pageSize: 50 });
    expect(store.decided.map(i => i.approval.id)).toEqual(['Ok1', 'No1']);
    await store.ensureLoaded(projectId, 'expired');
    expect(store.expired.map(i => i.approval.id)).toEqual(['Ex1']);
  });

  it('reads only the pending count for the sidebar, with one tiny page', async () => {
    const c = client();
    const store = useApprovalsStore();
    await store.loadCount(projectId);
    expect(c.listApprovals).toHaveBeenCalledWith({ projectId, status: 'pending', pageToken: '', pageSize: 1 });
    expect([store.pendingCount, store.countProject, store.projectId]).toEqual([3, projectId, '']);
    client({ listApprovals: vi.fn().mockRejectedValue(new Error('offline')) });
    await store.loadCount(projectId);
    expect(store.pendingCount).toBe(3);
  });

  it('never shows one project\'s approvals under another after the sidebar counts the new project', async () => {
    const c = client();
    const store = useApprovalsStore();
    await store.load(projectId, 'pending');
    await store.loadCount('B7rTn1YkM8wQz3XpL5vGd');
    expect(store.projectId).toBe(projectId);
    await store.ensureLoaded('B7rTn1YkM8wQz3XpL5vGd', 'pending');
    expect(c.listApprovals).toHaveBeenLastCalledWith({ projectId: 'B7rTn1YkM8wQz3XpL5vGd', status: 'pending', pageToken: '', pageSize: 50 });
    expect(store.projectId).toBe('B7rTn1YkM8wQz3XpL5vGd');
  });

  it('shows a load error and keeps nothing stale', async () => {
    client({ listApprovals: vi.fn().mockRejectedValue(new Error('offline')) });
    const store = useApprovalsStore();
    await store.load(projectId, 'pending');
    expect(store.error).toEqual({ message: 'error.load_approvals' });
    expect(store.pending).toEqual([]);
  });

  it('decides with the reason, moves the item to Decided and lowers the count', async () => {
    const c = client();
    const store = useApprovalsStore();
    await store.load(projectId, 'pending');
    expect(await store.decide('Ap1', false, ' salah ')).toEqual({ outcome: 'decided' });
    expect(c.decideApproval).toHaveBeenCalledWith({ id: 'Ap1', approved: false, text: 'salah' });
    expect(store.pending.map(i => i.approval.id)).toEqual(['Ap2']);
    expect(store.decided[0]!.approval).toMatchObject({ id: 'Ap1', status: 'rejected', reason: 'salah', decidedBy: 'Budi' });
    expect(store.pendingCount).toBe(2);
  });

  it('on APR003 shows what happened instead: decided elsewhere (refetched) or expired', async () => {
    const c = client({ decideApproval: vi.fn().mockRejectedValueOnce(connectError('APR003', { status: 'approved' })).mockRejectedValueOnce(connectError('APR003', { status: 'expired' })) });
    const store = useApprovalsStore();
    await store.load(projectId, 'pending');
    expect(await store.decide('Ap1', false, 'nope')).toEqual({ outcome: 'elsewhere' });
    expect(c.getApproval).toHaveBeenCalledWith({ id: 'Ap1' });
    expect(store.decided[0]!.approval).toMatchObject({ id: 'Ap1', status: 'approved', decidedBy: 'Sari', decidedVia: { channel: 'whatsapp' } });
    expect(await store.decide('Ap2', true)).toEqual({ outcome: 'expired' });
    expect(store.expired.map(i => i.approval.id)).toEqual(['Ap2']);
    expect(store.pending.map(i => [i.approval.id, i.approval.status])).toEqual([['Ap1', 'approved'], ['Ap2', 'expired']]);
    expect([...store.noted]).toEqual(['Ap1', 'Ap2']);
    expect(store.pendingCount).toBe(1);
    store.dismiss('Ap1');
    expect(store.pending.map(i => i.approval.id)).toEqual(['Ap2']);
    await store.refreshPending(new Set());
    expect(store.noted.size).toBe(0);
  });

  it('drops a card decided elsewhere when it can no longer be read', async () => {
    client({ decideApproval: vi.fn().mockRejectedValue(connectError('APR003', { status: 'rejected' })), getApproval: vi.fn().mockRejectedValue(new Error('offline')) });
    const store = useApprovalsStore();
    await store.load(projectId, 'pending');
    expect(await store.decide('Ap1', true)).toEqual({ outcome: 'elsewhere' });
    expect(store.pending.map(i => i.approval.id)).toEqual(['Ap2']);
  });

  it('drops an approval that is gone, refuses an app-answered one, and returns any other error', async () => {
    client({ decideApproval: vi.fn().mockRejectedValueOnce(connectError('APR002')).mockRejectedValueOnce(connectError('APR006')).mockRejectedValueOnce(new Error('offline')) });
    const store = useApprovalsStore();
    await store.load(projectId, 'pending');
    expect(await store.decide('Ap1', true)).toEqual({ outcome: 'gone' });
    expect(store.pending.map(i => i.approval.id)).toEqual(['Ap2']);
    expect((await store.decide('Ap2', true)).outcome).toBe('app');
    const failed = await store.decide('Ap2', true);
    expect(failed.outcome).toBe('error');
    expect(failed.error).toEqual({ message: 'error.decide_approval' });
    expect(await store.decide('nope', true)).toEqual({ outcome: 'gone' });
  });

  it('refreshes pending in place: adds new ones at the top, drops ones decided elsewhere, and leaves kept cards alone', async () => {
    const c = client();
    const store = useApprovalsStore();
    await store.load(projectId, 'pending');
    const kept = store.pending[1];
    c.listApprovals.mockResolvedValueOnce({ approvals: [pb('New1'), pb('Ap1')], nextPageToken: '', pendingCount: 4n });
    expect(await store.refreshPending(new Set(['Ap2']))).toBe(1);
    expect(store.pending.map(i => i.approval.id)).toEqual(['New1', 'Ap1', 'Ap2']);
    expect(store.pending[2]).toBe(kept);
    expect(store.pendingCount).toBe(4);
    c.listApprovals.mockRejectedValueOnce(new Error('offline'));
    expect(await store.refreshPending(new Set())).toBe(0);
    expect(await useApprovalsStore().refreshPending(new Set())).toBe(0);
  });
});
