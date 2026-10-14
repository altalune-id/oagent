import { create } from '@bufbuild/protobuf';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ref } from 'vue';

import { useInvitesStore } from '~/stores/invites';
import { InviteSchema } from '../../../../gen/invite/v1/invite_pb';
import { connectError, setClient, setKnownKeys } from '../../../../test/nuxt-globals';
import { useInvite } from './useInvite';

const bob = create(InviteSchema, { id: '2MPkgPSKGTZpjjYvbdNZd', email: 'bob@x.io', role: 'admin', status: 'pending', invitedByName: 'Olga' });
const dina = create(InviteSchema, { id: 'J2zxkPfqntGKCKysV3nfh', email: 'dina@x.io', role: 'member', status: 'draft', invitedByName: 'Olga' });

describe('useInvite', () => {
  let client: Record<string, ReturnType<typeof vi.fn>>;

  beforeEach(() => {
    setActivePinia(createPinia());
    client = {
      listInvites: vi.fn().mockResolvedValue({ invites: [bob], drafts: [dina] }),
      createInvite: vi.fn().mockResolvedValue({ invite: create(InviteSchema, { id: 'uqTpkuXzc6PSempkGCvX3', email: 'cy@x.io', role: 'member', status: 'pending' }), link: 'https://x/invite/t3' }),
      resendInvite: vi.fn().mockResolvedValue({ invite: bob, link: 'https://x/invite/t1b' }),
      revokeInvite: vi.fn().mockResolvedValue({}),
      confirmDraftInvite: vi.fn().mockResolvedValue({ invite: create(InviteSchema, { ...dina, status: 'pending' }) }),
      discardDraftInvite: vi.fn().mockResolvedValue({}),
    };
    setClient('$inviteClient', client);
    setKnownKeys();
  });

  async function loaded(role = 'owner') {
    const viewerRole = ref(role);
    const inv = useInvite('CEJfSG6fWkpY2S4t2Y24G', viewerRole);
    await inv.load();
    return { inv, viewerRole };
  }

  it('offers the roles the viewer may invite to', async () => {
    const { inv, viewerRole } = await loaded('admin');
    expect(inv.roles.value).toEqual(['admin', 'member']);
    expect(inv.canInvite.value).toBe(true);
    viewerRole.value = 'member';
    expect(inv.roles.value).toEqual([]);
    expect(inv.canInvite.value).toBe(false);
    viewerRole.value = 'owner';
    expect(inv.roles.value).toEqual(['owner', 'admin', 'member']);
  });

  it('sends an invite with the org public id and shows the link once', async () => {
    const { inv } = await loaded();
    inv.open();
    expect(inv.modalOpen.value).toBe(true);
    inv.email.value = ' cy@x.io ';
    await inv.submit();
    expect(client.createInvite).toHaveBeenCalledWith({ orgId: 'CEJfSG6fWkpY2S4t2Y24G', email: 'cy@x.io', role: 'member' });
    expect(inv.sent.value).toEqual({ id: 'uqTpkuXzc6PSempkGCvX3', email: 'cy@x.io', link: 'https://x/invite/t3' });
    expect(inv.sentMessage.value).toBe('invites.emailed|{"email":"cy@x.io"}');
    expect(useInvitesStore().pending.map(i => i.id)).toEqual(['2MPkgPSKGTZpjjYvbdNZd', 'uqTpkuXzc6PSempkGCvX3']);
    inv.close();
    expect(inv.modalOpen.value).toBe(false);
    inv.open();
    expect(inv.sent.value).toBeNull();
    expect(inv.email.value).toBe('');
    expect(inv.sentMessage.value).toBe('');
  });

  it('shows translated errors, like an admin inviting an owner', async () => {
    setKnownKeys('apperror.MBR005', 'apperror.USR003');
    const { inv } = await loaded('admin');
    inv.open();
    inv.email.value = 'cy@x.io';
    inv.role.value = 'owner';
    client.createInvite!.mockRejectedValueOnce(connectError('MBR005', { role: 'owner' }))
      .mockRejectedValueOnce(connectError('USR003'))
      .mockRejectedValueOnce(new Error('boom'));
    await inv.submit();
    expect(inv.error.value).toEqual({ message: 'apperror.MBR005|{"role":"owner"}', reference: 'error.ref|{"id":"req-1"}' });
    expect(inv.sent.value).toBeNull();
    await inv.submit();
    expect(inv.error.value).toBeNull();
    expect(inv.fieldErrors.value, 'an email the server refuses shows under the email input').toEqual({ email: 'apperror.USR003' });
    await inv.submit();
    expect(inv.error.value).toEqual({ message: 'error.create_invite' });
    expect(inv.busy.value).toBe(false);
  });

  it('ignores a second submit while one runs, and an empty email', async () => {
    const { inv } = await loaded();
    await inv.submit();
    expect(client.createInvite).not.toHaveBeenCalled();
    inv.email.value = 'cy@x.io';
    const a = inv.submit();
    const b = inv.submit();
    await Promise.all([a, b]);
    expect(client.createInvite).toHaveBeenCalledTimes(1);
  });

  it('resends and shows the new link by the row', async () => {
    const { inv } = await loaded();
    await inv.resend('2MPkgPSKGTZpjjYvbdNZd');
    expect(client.resendInvite).toHaveBeenCalledWith({ orgId: 'CEJfSG6fWkpY2S4t2Y24G', inviteId: '2MPkgPSKGTZpjjYvbdNZd' });
    expect(inv.rowLink.value).toEqual({ id: '2MPkgPSKGTZpjjYvbdNZd', email: 'bob@x.io', link: 'https://x/invite/t1b' });
    client.resendInvite!.mockRejectedValueOnce(new Error('boom'));
    await inv.resend('2MPkgPSKGTZpjjYvbdNZd');
    expect(inv.error.value).toEqual({ message: 'error.resend_invite' });
  });

  it('asks before revoking, then drops the row', async () => {
    const { inv } = await loaded();
    inv.askRevoke('2MPkgPSKGTZpjjYvbdNZd');
    expect(client.revokeInvite).not.toHaveBeenCalled();
    expect(inv.confirming.value).toEqual({ id: '2MPkgPSKGTZpjjYvbdNZd', email: 'bob@x.io' });
    expect(inv.confirmMessage.value).toBe('invites.revoke_confirm|{"email":"bob@x.io"}');
    inv.cancel();
    expect(inv.confirming.value).toBeNull();
    expect(inv.confirmMessage.value).toBe('');
    inv.askRevoke('nope');
    expect(inv.confirming.value).toBeNull();
    await inv.confirm();
    expect(client.revokeInvite).not.toHaveBeenCalled();

    inv.askRevoke('2MPkgPSKGTZpjjYvbdNZd');
    await inv.confirm();
    expect(client.revokeInvite).toHaveBeenCalledWith({ orgId: 'CEJfSG6fWkpY2S4t2Y24G', inviteId: '2MPkgPSKGTZpjjYvbdNZd' });
    expect(useInvitesStore().pending).toEqual([]);

    await useInvitesStore().load('CEJfSG6fWkpY2S4t2Y24G');
    client.revokeInvite!.mockRejectedValueOnce(new Error('boom'));
    inv.askRevoke('2MPkgPSKGTZpjjYvbdNZd');
    await inv.confirm();
    expect(inv.error.value).toEqual({ message: 'error.revoke_invite' });
  });

  it('confirms a draft an assistant made and says it was emailed, with no link', async () => {
    const { inv } = await loaded();
    await inv.confirmDraft('J2zxkPfqntGKCKysV3nfh');
    expect(client.confirmDraftInvite).toHaveBeenCalledWith({ orgId: 'CEJfSG6fWkpY2S4t2Y24G', inviteId: 'J2zxkPfqntGKCKysV3nfh' });
    expect(inv.rowLink.value).toBeNull();
    expect(inv.notice.value).toBe('invites.sent_to|{"email":"dina@x.io"}');
    expect(useInvitesStore().drafts).toEqual([]);
    expect(useInvitesStore().pending.map(i => i.id)).toEqual(['2MPkgPSKGTZpjjYvbdNZd', 'J2zxkPfqntGKCKysV3nfh']);
  });

  it('reads an expired draft from the server', async () => {
    setKnownKeys('apperror.INV003');
    const { inv } = await loaded();
    client.confirmDraftInvite!.mockRejectedValueOnce(connectError('INV003'));
    await inv.confirmDraft('J2zxkPfqntGKCKysV3nfh');
    expect(inv.error.value?.message).toBe('apperror.INV003');
    expect(inv.rowLink.value).toBeNull();
    expect(inv.notice.value).toBe('');
    expect(useInvitesStore().drafts).toEqual([]);
  });

  it('keeps a draft when confirming fails for another reason', async () => {
    const { inv } = await loaded();
    client.confirmDraftInvite!.mockRejectedValueOnce(new Error('boom'));
    await inv.confirmDraft('J2zxkPfqntGKCKysV3nfh');
    expect(inv.error.value).toEqual({ message: 'error.confirm_invite' });
    expect(useInvitesStore().drafts.map(i => i.id)).toEqual(['J2zxkPfqntGKCKysV3nfh']);
  });

  it('clears a failed send error when the modal closes', async () => {
    const { inv } = await loaded();
    inv.open();
    inv.email.value = 'cy@x.io';
    client.createInvite!.mockRejectedValueOnce(new Error('boom'));
    await inv.submit();
    expect(inv.error.value).toEqual({ message: 'error.create_invite' });
    inv.close();
    expect(inv.error.value).toBeNull();
  });

  it('discards a draft', async () => {
    const { inv } = await loaded();
    await inv.discardDraft('J2zxkPfqntGKCKysV3nfh');
    expect(client.discardDraftInvite).toHaveBeenCalledWith({ orgId: 'CEJfSG6fWkpY2S4t2Y24G', inviteId: 'J2zxkPfqntGKCKysV3nfh' });
    expect(useInvitesStore().drafts).toEqual([]);
    client.discardDraftInvite!.mockRejectedValueOnce(new Error('boom'));
    await inv.discardDraft('J2zxkPfqntGKCKysV3nfh');
    expect(inv.error.value).toEqual({ message: 'error.discard_invite' });
  });

  it('exposes the store state', async () => {
    const { inv } = await loaded();
    expect(inv.pending.value.map(i => i.id)).toEqual(['2MPkgPSKGTZpjjYvbdNZd']);
    expect(inv.drafts.value.map(i => i.id)).toEqual(['J2zxkPfqntGKCKysV3nfh']);
    expect(inv.loading.value).toBe(false);
    expect(inv.loadError.value).toBeNull();
  });

  it('checks the email before sending', async () => {
    const { inv } = await loaded('admin');
    inv.open();
    inv.email.value = 'cy at x.io';
    await inv.submit();
    expect(client.createInvite).not.toHaveBeenCalled();
    expect(inv.fieldErrors.value).toEqual({ email: 'validation.invalid_format' });
    inv.close();
    expect(inv.fieldErrors.value).toEqual({});
  });
});
