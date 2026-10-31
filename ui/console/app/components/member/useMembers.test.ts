import { create } from '@bufbuild/protobuf';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { MemberSchema } from '#gen/member/v1/member_pb';
import { connectError, deferred, navigateTo, setClient, setKnownKeys } from '#test/nuxt-globals';
import { useMembersStore } from '~/stores/members';
import { useMembers } from './useMembers';

const owner = create(MemberSchema, { userId: 'FtGrdYskgDVDf8apKsMdn', email: 'ann@example.com', name: 'Ann Lee', role: 'owner' });
const admin = create(MemberSchema, { userId: '5jeWP8ut8V9AC9GBMMHfx', email: 'bob@example.com', name: '', role: 'admin' });
const member = create(MemberSchema, { userId: 'jzdsjzt2UHJ3Cmr3uN2VP', email: 'cy@example.com', name: 'Cy', role: 'member' });

describe('useMembers', () => {
  let client: Record<string, ReturnType<typeof vi.fn>>;
  let whoami: ReturnType<typeof vi.fn>;
  let listOrgs: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    setActivePinia(createPinia());
    client = {
      listMembers: vi.fn().mockResolvedValue({ members: [owner, admin, member] }),
      setMemberRole: vi.fn().mockResolvedValue({ member: create(MemberSchema, { ...member, role: 'admin' }) }),
      removeMember: vi.fn().mockResolvedValue({}),
      leaveOrg: vi.fn().mockResolvedValue({}),
    };
    whoami = vi.fn().mockResolvedValue({ userId: '5jeWP8ut8V9AC9GBMMHfx', source: 'local' });
    listOrgs = vi.fn().mockResolvedValue({ orgs: [] });
    setClient('$memberClient', client);
    setClient('$authClient', { whoami });
    setClient('$orgClient', { listOrgs });
  });

  async function loaded(orgId = 'V3StGXR8Z5jdHj6BmyT2a') {
    const m = useMembers(orgId);
    await m.load();
    return m;
  }

  it('loads the viewer and the members, with what the viewer may do on each row', async () => {
    const m = useMembers('V3StGXR8Z5jdHj6BmyT2a');
    expect(m.rows.value).toEqual([]);
    expect(m.canLeave.value).toBe(false);
    await m.load();
    expect(client.listMembers).toHaveBeenCalledWith({ orgId: 'V3StGXR8Z5jdHj6BmyT2a' });
    expect(m.viewer.value).toEqual({ userId: '5jeWP8ut8V9AC9GBMMHfx', role: 'admin' });
    expect(m.canLeave.value).toBe(true);
    expect(m.rows.value.map(r => [r.userId, r.initials, r.self, r.roles, r.removable])).toEqual([
      ['FtGrdYskgDVDf8apKsMdn', 'AL', false, [], false],
      ['5jeWP8ut8V9AC9GBMMHfx', 'B', true, [], false],
      ['jzdsjzt2UHJ3Cmr3uN2VP', 'C', false, ['admin', 'member'], true],
    ]);
  });

  it('gives a signed-out viewer no actions', async () => {
    whoami.mockRejectedValue(new Error('anonymous'));
    const m = await loaded();
    expect(m.viewer.value).toEqual({ userId: '', role: '' });
    expect(m.canLeave.value).toBe(false);
    expect(m.rows.value.every(r => r.roles.length === 0 && !r.removable && !r.self)).toBe(true);
  });

  it('shows load errors from the store', async () => {
    client.listMembers!.mockRejectedValue(new Error('boom'));
    const m = await loaded();
    expect(m.loadError.value).toEqual({ message: 'error.load_members' });
    expect(m.loading.value).toBe(false);
  });

  async function changeRole(m: ReturnType<typeof useMembers>, userId: string, role: string) {
    m.askRole(userId, role);
    await m.confirm();
  }

  it('asks before changing a role, naming the person and both roles', async () => {
    const m = await loaded();
    m.askRole('jzdsjzt2UHJ3Cmr3uN2VP', 'admin');
    expect(client.setMemberRole).not.toHaveBeenCalled();
    expect(m.confirming.value).toEqual({ kind: 'role', userId: 'jzdsjzt2UHJ3Cmr3uN2VP', name: 'Cy', from: 'member', to: 'admin' });
    expect(m.confirmMessage.value).toBe('members.role_confirm|{"name":"Cy","from":"members.role_member","to":"members.role_admin"}');
    m.cancel();
    expect(m.confirming.value).toBeNull();
    expect(useMembersStore().byUser('jzdsjzt2UHJ3Cmr3uN2VP')?.role).toBe('member');
    expect(client.setMemberRole).not.toHaveBeenCalled();
  });

  it('does not ask for an unknown member or the same role', async () => {
    const m = await loaded();
    m.askRole('nobody', 'admin');
    expect(m.confirming.value).toBeNull();
    m.askRole('jzdsjzt2UHJ3Cmr3uN2VP', 'member');
    expect(m.confirming.value).toBeNull();
  });

  it('changes a role on confirm and keeps the server copy', async () => {
    const pending = deferred<unknown>();
    client.setMemberRole!.mockReturnValue(pending.promise);
    const m = await loaded();
    m.askRole('jzdsjzt2UHJ3Cmr3uN2VP', 'admin');
    const run = m.confirm();
    expect(m.confirming.value).toBeNull();
    expect(useMembersStore().byUser('jzdsjzt2UHJ3Cmr3uN2VP')?.role).toBe('admin');
    expect(m.busy.value).toBe(true);
    pending.resolve({ member: create(MemberSchema, { ...member, name: 'Cy Fresh', role: 'admin' }) });
    await run;
    expect(client.setMemberRole).toHaveBeenCalledWith({ orgId: 'V3StGXR8Z5jdHj6BmyT2a', userId: 'jzdsjzt2UHJ3Cmr3uN2VP', role: 'admin' });
    expect(useMembersStore().byUser('jzdsjzt2UHJ3Cmr3uN2VP')).toMatchObject({ name: 'Cy Fresh', role: 'admin' });
    expect(m.busy.value).toBe(false);
    expect(m.error.value).toBeNull();
  });

  it('keeps the optimistic role when the server sends no member back', async () => {
    client.setMemberRole!.mockResolvedValue({});
    const m = await loaded();
    await changeRole(m, 'jzdsjzt2UHJ3Cmr3uN2VP', 'admin');
    expect(useMembersStore().byUser('jzdsjzt2UHJ3Cmr3uN2VP')?.role).toBe('admin');
  });

  it('rolls a role change back and shows the error', async () => {
    setKnownKeys('apperror.MBR005');
    client.setMemberRole!.mockRejectedValueOnce(connectError('MBR005', { role: 'owner' })).mockRejectedValueOnce(new Error('boom'));
    const m = await loaded();
    await changeRole(m, 'jzdsjzt2UHJ3Cmr3uN2VP', 'admin');
    expect(useMembersStore().byUser('jzdsjzt2UHJ3Cmr3uN2VP')?.role).toBe('member');
    expect(m.error.value).toEqual({ message: 'apperror.MBR005|{"role":"owner"}', reference: 'error.ref|{"id":"req-1"}' });
    await changeRole(m, 'jzdsjzt2UHJ3Cmr3uN2VP', 'admin');
    expect(m.error.value).toEqual({ message: 'error.set_member_role' });
    expect(m.busy.value).toBe(false);
  });

  it('skips a role change whose member is gone, and a confirm while busy', async () => {
    const pending = deferred<unknown>();
    client.setMemberRole!.mockReturnValue(pending.promise);
    const m = await loaded();
    m.askRole('jzdsjzt2UHJ3Cmr3uN2VP', 'admin');
    useMembersStore().drop('V3StGXR8Z5jdHj6BmyT2a', 'jzdsjzt2UHJ3Cmr3uN2VP');
    await m.confirm();
    expect(client.setMemberRole).not.toHaveBeenCalled();
    await m.load();
    m.askRole('jzdsjzt2UHJ3Cmr3uN2VP', 'admin');
    const run = m.confirm();
    m.askRole('FtGrdYskgDVDf8apKsMdn', 'admin');
    await m.confirm();
    expect(m.confirming.value).toEqual({ kind: 'role', userId: 'FtGrdYskgDVDf8apKsMdn', name: 'Ann Lee', from: 'owner', to: 'admin' });
    pending.resolve({});
    await run;
    expect(client.setMemberRole).toHaveBeenCalledTimes(1);
  });

  describe('after switching to another org', () => {
    const other = create(MemberSchema, { ...member, name: 'Cy Two' });

    async function switched() {
      client.listMembers!.mockResolvedValueOnce({ members: [owner, admin, member] }).mockResolvedValueOnce({ members: [admin, other] });
      const a = await loaded('V3StGXR8Z5jdHj6BmyT2a');
      return { a, b: () => loaded('YeDxg2kMYznPJJG3wVmjj') };
    }

    it('ignores a late role reply from the old org', async () => {
      const pending = deferred<unknown>();
      client.setMemberRole!.mockReturnValue(pending.promise);
      const { a, b } = await switched();
      a.askRole('jzdsjzt2UHJ3Cmr3uN2VP', 'admin');
      const run = a.confirm();
      await b();
      pending.resolve({ member: create(MemberSchema, { ...member, role: 'admin' }) });
      await run;
      expect(useMembersStore().byUser('jzdsjzt2UHJ3Cmr3uN2VP')).toMatchObject({ name: 'Cy Two', role: 'member' });
    });

    it('ignores a role rollback from the old org', async () => {
      const pending = deferred<unknown>();
      client.setMemberRole!.mockReturnValue(pending.promise);
      const { a, b } = await switched();
      a.askRole('jzdsjzt2UHJ3Cmr3uN2VP', 'admin');
      const run = a.confirm();
      await b();
      useMembersStore().replace('YeDxg2kMYznPJJG3wVmjj', { ...useMembersStore().byUser('jzdsjzt2UHJ3Cmr3uN2VP')!, role: 'admin' });
      pending.reject(new Error('boom'));
      await run;
      expect(useMembersStore().byUser('jzdsjzt2UHJ3Cmr3uN2VP')).toMatchObject({ name: 'Cy Two', role: 'admin' });
      expect(a.error.value).toEqual({ message: 'error.set_member_role' });
    });

    it('ignores a late remove from the old org', async () => {
      const pending = deferred<unknown>();
      client.removeMember!.mockReturnValue(pending.promise);
      const { a, b } = await switched();
      a.askRemove('jzdsjzt2UHJ3Cmr3uN2VP');
      const run = a.confirm();
      await b();
      pending.resolve({});
      await run;
      expect(useMembersStore().byUser('jzdsjzt2UHJ3Cmr3uN2VP')).toMatchObject({ name: 'Cy Two' });
    });
  });

  it('asks before removing, naming the person, and removes on confirm', async () => {
    const m = await loaded();
    m.askRemove('jzdsjzt2UHJ3Cmr3uN2VP');
    expect(client.removeMember).not.toHaveBeenCalled();
    expect(m.confirming.value).toEqual({ kind: 'remove', userId: 'jzdsjzt2UHJ3Cmr3uN2VP', name: 'Cy' });
    expect(m.confirmMessage.value).toBe('members.remove_confirm|{"name":"Cy"}');
    await m.confirm();
    expect(client.removeMember).toHaveBeenCalledWith({ orgId: 'V3StGXR8Z5jdHj6BmyT2a', userId: 'jzdsjzt2UHJ3Cmr3uN2VP' });
    expect(useMembersStore().byUser('jzdsjzt2UHJ3Cmr3uN2VP')).toBeUndefined();
    expect(m.confirming.value).toBeNull();
    expect(m.busy.value).toBe(false);
  });

  it('names a person without a name by email, and ignores an unknown one', async () => {
    whoami.mockResolvedValue({ userId: 'FtGrdYskgDVDf8apKsMdn' });
    const m = await loaded();
    m.askRemove('nobody');
    expect(m.confirming.value).toBeNull();
    expect(m.confirmMessage.value).toBe('');
    m.askRemove('5jeWP8ut8V9AC9GBMMHfx');
    expect(m.confirming.value).toEqual({ kind: 'remove', userId: '5jeWP8ut8V9AC9GBMMHfx', name: 'bob@example.com' });
    m.cancel();
    expect(m.confirming.value).toBeNull();
  });

  it('shows remove errors and keeps the member', async () => {
    setKnownKeys('apperror.GEN900');
    client.removeMember!.mockRejectedValueOnce(connectError('GEN900')).mockRejectedValueOnce(new Error('boom'));
    const m = await loaded();
    m.askRemove('jzdsjzt2UHJ3Cmr3uN2VP');
    await m.confirm();
    expect(m.error.value).toEqual({ message: 'apperror.GEN900', reference: 'error.ref|{"id":"req-1"}' });
    expect(useMembersStore().byUser('jzdsjzt2UHJ3Cmr3uN2VP')).toBeDefined();
    expect(m.confirming.value).toBeNull();
    m.askRemove('jzdsjzt2UHJ3Cmr3uN2VP');
    await m.confirm();
    expect(m.error.value).toEqual({ message: 'error.remove_member' });
  });

  it('asks before leaving, then leaves, reloads the orgs and goes to /orgs', async () => {
    const m = await loaded();
    m.askLeave();
    expect(m.confirming.value).toEqual({ kind: 'leave', userId: '5jeWP8ut8V9AC9GBMMHfx', name: 'bob@example.com' });
    expect(m.confirmMessage.value).toBe('members.leave_confirm');
    expect(client.leaveOrg).not.toHaveBeenCalled();
    await m.confirm();
    expect(client.leaveOrg).toHaveBeenCalledWith({ orgId: 'V3StGXR8Z5jdHj6BmyT2a' });
    expect(listOrgs).toHaveBeenCalled();
    expect(navigateTo).toHaveBeenCalledWith('/orgs');
    expect(m.confirming.value).toBeNull();
  });

  it('does not ask a non-member to leave', async () => {
    whoami.mockResolvedValue({ userId: 'X3VTKFPS6cGX2PnS2FH5J' });
    const m = await loaded();
    m.askLeave();
    expect(m.confirming.value).toBeNull();
  });

  it('shows leave errors and stays on the page', async () => {
    setKnownKeys('apperror.MBR004');
    client.leaveOrg!.mockRejectedValueOnce(connectError('MBR004')).mockRejectedValueOnce(new Error('boom'));
    const m = await loaded();
    m.askLeave();
    await m.confirm();
    expect(m.error.value).toEqual({ message: 'apperror.MBR004', reference: 'error.ref|{"id":"req-1"}' });
    expect(navigateTo).not.toHaveBeenCalled();
    m.askLeave();
    await m.confirm();
    expect(m.error.value).toEqual({ message: 'error.leave_org' });
  });

  it('does nothing on confirm with nothing to confirm', async () => {
    const m = await loaded();
    await m.confirm();
    expect(client.removeMember).not.toHaveBeenCalled();
    expect(client.leaveOrg).not.toHaveBeenCalled();
  });
});
