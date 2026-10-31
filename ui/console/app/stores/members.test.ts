import type { Member } from './members';
import { create } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { createPinia, setActivePinia } from 'pinia';

import { beforeEach, describe, expect, it, vi } from 'vitest';
import { MemberSchema } from '#gen/member/v1/member_pb';
import { connectError, deferred, setClient, setKnownKeys } from '#test/nuxt-globals';
import { toMember, useMembersStore } from './members';

const joined = new Date('2026-09-01T00:00:00Z');
const annPB = create(MemberSchema, { userId: 'FtGrdYskgDVDf8apKsMdn', email: 'ann@example.com', name: 'Ann Lee', role: 'owner', joinedAt: timestampFromDate(joined) });
const bobPB = create(MemberSchema, { userId: '5jeWP8ut8V9AC9GBMMHfx', email: 'bob@example.com', name: '', role: 'member' });
const ann: Member = { userId: 'FtGrdYskgDVDf8apKsMdn', email: 'ann@example.com', name: 'Ann Lee', role: 'owner', joinedAt: joined };
const bob: Member = { userId: '5jeWP8ut8V9AC9GBMMHfx', email: 'bob@example.com', name: '', role: 'member', joinedAt: undefined };

describe('toMember', () => {
  it('converts the join time to a date', () => {
    expect(toMember(annPB)).toEqual(ann);
    expect(toMember(bobPB)).toEqual(bob);
  });
});

describe('useMembersStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('loads the members of an org', async () => {
    const listMembers = vi.fn().mockResolvedValue({ members: [annPB, bobPB] });
    setClient('$memberClient', { listMembers });
    const members = useMembersStore();
    await members.load('V3StGXR8Z5jdHj6BmyT2a');
    expect(listMembers).toHaveBeenCalledWith({ orgId: 'V3StGXR8Z5jdHj6BmyT2a' });
    expect(members.items).toEqual([ann, bob]);
    expect(members.loading).toBe(false);
    expect(members.byUser('5jeWP8ut8V9AC9GBMMHfx')).toEqual(bob);
    expect(members.byUser('nobody')).toBeUndefined();
  });

  it('treats a missing list as empty', async () => {
    setClient('$memberClient', { listMembers: vi.fn().mockResolvedValue({}) });
    const members = useMembersStore();
    await members.load('V3StGXR8Z5jdHj6BmyT2a');
    expect(members.items).toEqual([]);
  });

  it('shows a Connect error with its reference, and the fallback for a plain error', async () => {
    setKnownKeys('apperror.GEN900');
    const listMembers = vi.fn().mockRejectedValueOnce(connectError('GEN900')).mockRejectedValueOnce(new Error('boom'));
    setClient('$memberClient', { listMembers });
    const members = useMembersStore();
    await members.load('V3StGXR8Z5jdHj6BmyT2a');
    expect(members.error).toEqual({ message: 'apperror.GEN900', reference: 'error.ref|{"id":"req-1"}' });
    await members.load('V3StGXR8Z5jdHj6BmyT2a');
    expect(members.error).toEqual({ message: 'error.load_members' });
    expect(members.loading).toBe(false);
  });

  it('keeps only the latest load when an older one finishes last', async () => {
    const first = deferred<{ members: typeof annPB[] }>();
    const second = deferred<{ members: typeof annPB[] }>();
    const listMembers = vi.fn().mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
    setClient('$memberClient', { listMembers });
    const members = useMembersStore();
    const a = members.load('V3StGXR8Z5jdHj6BmyT2a');
    const b = members.load('YeDxg2kMYznPJJG3wVmjj');
    second.resolve({ members: [bobPB] });
    await b;
    first.resolve({ members: [annPB] });
    await a;
    expect(members.items).toEqual([bob]);
    expect(members.loading).toBe(false);
  });

  it('ignores an error from an older load', async () => {
    const first = deferred<{ members: typeof annPB[] }>();
    const listMembers = vi.fn().mockReturnValueOnce(first.promise).mockResolvedValueOnce({ members: [annPB] });
    setClient('$memberClient', { listMembers });
    const members = useMembersStore();
    const a = members.load('V3StGXR8Z5jdHj6BmyT2a');
    await members.load('V3StGXR8Z5jdHj6BmyT2a');
    first.reject(new Error('late'));
    await a;
    expect(members.error).toBeNull();
    expect(members.items).toEqual([ann]);
  });

  it('replaces and drops members of the loaded org', async () => {
    setClient('$memberClient', { listMembers: vi.fn().mockResolvedValue({ members: [annPB, bobPB] }) });
    const members = useMembersStore();
    await members.load('V3StGXR8Z5jdHj6BmyT2a');
    members.replace('V3StGXR8Z5jdHj6BmyT2a', { ...bob, role: 'admin' });
    expect(members.byUser('5jeWP8ut8V9AC9GBMMHfx')?.role).toBe('admin');
    members.replace('V3StGXR8Z5jdHj6BmyT2a', { ...bob, userId: 'jzdsjzt2UHJ3Cmr3uN2VP' });
    expect(members.items.map(m => m.userId)).toEqual(['FtGrdYskgDVDf8apKsMdn', '5jeWP8ut8V9AC9GBMMHfx']);
    members.drop('V3StGXR8Z5jdHj6BmyT2a', 'FtGrdYskgDVDf8apKsMdn');
    expect(members.items.map(m => m.userId)).toEqual(['5jeWP8ut8V9AC9GBMMHfx']);
  });

  it('ignores a replace or drop for another org after the list switched', async () => {
    const listMembers = vi.fn().mockResolvedValue({ members: [annPB, bobPB] });
    setClient('$memberClient', { listMembers });
    const members = useMembersStore();
    await members.load('V3StGXR8Z5jdHj6BmyT2a');
    await members.load('YeDxg2kMYznPJJG3wVmjj');
    members.replace('V3StGXR8Z5jdHj6BmyT2a', { ...bob, role: 'admin' });
    members.drop('V3StGXR8Z5jdHj6BmyT2a', 'FtGrdYskgDVDf8apKsMdn');
    expect(members.items).toEqual([ann, bob]);
  });
});
