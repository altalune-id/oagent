import { create } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { InviteSchema } from '../../gen/invite/v1/invite_pb';
import { deferred, setClient, setKnownKeys } from '../../test/nuxt-globals';
import { toInvite, useInvitesStore } from './invites';

const expires = new Date('2026-10-20T09:00:00Z');
const bobPB = create(InviteSchema, { id: '2MPkgPSKGTZpjjYvbdNZd', email: 'bob@x.io', role: 'admin', status: 'pending', invitedByName: 'Olga', expiresAt: timestampFromDate(expires) });
const dinaPB = create(InviteSchema, { id: 'J2zxkPfqntGKCKysV3nfh', email: 'dina@x.io', role: 'member', status: 'draft' });

describe('toInvite', () => {
  it('converts the dates', () => {
    expect(toInvite(bobPB)).toEqual({ id: '2MPkgPSKGTZpjjYvbdNZd', email: 'bob@x.io', role: 'admin', status: 'pending', invitedByName: 'Olga', expiresAt: expires, createdAt: undefined });
    expect(toInvite(dinaPB).expiresAt).toBeUndefined();
  });
});

describe('useInvitesStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('loads pending invites and drafts by the org public id', async () => {
    const listInvites = vi.fn().mockResolvedValue({ invites: [bobPB], drafts: [dinaPB] });
    setClient('$inviteClient', { listInvites });
    const s = useInvitesStore();
    await s.load('CEJfSG6fWkpY2S4t2Y24G');
    expect(listInvites).toHaveBeenCalledWith({ orgId: 'CEJfSG6fWkpY2S4t2Y24G' });
    expect(s.pending.map(i => i.id)).toEqual(['2MPkgPSKGTZpjjYvbdNZd']);
    expect(s.drafts.map(i => i.id)).toEqual(['J2zxkPfqntGKCKysV3nfh']);
    expect(s.loading).toBe(false);
  });

  it('reads a missing list as empty', async () => {
    setClient('$inviteClient', { listInvites: vi.fn().mockResolvedValue({}) });
    const s = useInvitesStore();
    await s.load('o');
    expect(s.pending).toEqual([]);
    expect(s.drafts).toEqual([]);
  });

  it('shows a load error', async () => {
    setKnownKeys();
    setClient('$inviteClient', { listInvites: vi.fn().mockRejectedValue(new Error('boom')) });
    const s = useInvitesStore();
    await s.load('o');
    expect(s.error).toEqual({ message: 'error.load_invites' });
    expect(s.loading).toBe(false);
  });

  it('never shows another org\'s invites when a load fails', async () => {
    setKnownKeys();
    const listInvites = vi.fn().mockResolvedValueOnce({ invites: [bobPB], drafts: [dinaPB] }).mockRejectedValueOnce(new Error('boom'));
    setClient('$inviteClient', { listInvites });
    const s = useInvitesStore();
    await s.load('orgA');
    expect(s.pending).toHaveLength(1);
    await s.load('orgB');
    expect(s.pending).toEqual([]);
    expect(s.drafts).toEqual([]);
    expect(s.error).toEqual({ message: 'error.load_invites' });
    s.upsert('orgA', toInvite(bobPB));
    expect(s.pending).toEqual([]);
  });

  it('keeps only the latest load', async () => {
    const first = deferred<unknown>();
    const listInvites = vi.fn().mockReturnValueOnce(first.promise).mockResolvedValueOnce({ invites: [bobPB] });
    setClient('$inviteClient', { listInvites });
    const s = useInvitesStore();
    const a = s.load('V3StGXR8Z5jdHj6BmyT2a');
    await s.load('YeDxg2kMYznPJJG3wVmjj');
    first.reject(new Error('late'));
    await a;
    expect(s.pending.map(i => i.id)).toEqual(['2MPkgPSKGTZpjjYvbdNZd']);
    expect(s.error).toBeNull();
  });

  it('moves a confirmed draft to pending, replaces a resent invite and drops one', async () => {
    setClient('$inviteClient', { listInvites: vi.fn().mockResolvedValue({ invites: [bobPB], drafts: [dinaPB] }) });
    const s = useInvitesStore();
    await s.load('V3StGXR8Z5jdHj6BmyT2a');
    s.upsert('V3StGXR8Z5jdHj6BmyT2a', { ...toInvite(dinaPB), status: 'pending' });
    expect(s.drafts).toEqual([]);
    expect(s.pending.map(i => [i.id, i.status])).toEqual([['2MPkgPSKGTZpjjYvbdNZd', 'pending'], ['J2zxkPfqntGKCKysV3nfh', 'pending']]);
    s.upsert('V3StGXR8Z5jdHj6BmyT2a', { ...toInvite(bobPB), role: 'member' });
    expect(s.pending[0]?.role).toBe('member');
    s.upsert('other', toInvite(bobPB));
    s.drop('other', '2MPkgPSKGTZpjjYvbdNZd');
    expect(s.pending).toHaveLength(2);
    s.drop('V3StGXR8Z5jdHj6BmyT2a', '2MPkgPSKGTZpjjYvbdNZd');
    expect(s.pending.map(i => i.id)).toEqual(['J2zxkPfqntGKCKysV3nfh']);
  });
});
