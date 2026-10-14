import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { useAuthStore } from '~/stores/auth';
import { connectError, navigateTo, runtimeConfig, setClient, setKnownKeys } from '../../../test/nuxt-globals';
import { useProfile } from './useProfile';

describe('useProfile', () => {
  let client: Record<string, ReturnType<typeof vi.fn>>;

  beforeEach(() => {
    setActivePinia(createPinia());
    client = {
      whoami: vi.fn().mockResolvedValue({ userId: 'V3StGXR8Z5jdHj6BmyT2a', source: 'local', name: 'Ana', email: 'ana@example.com', avatarUrl: '' }),
      updateProfile: vi.fn().mockResolvedValue({ userId: 'V3StGXR8Z5jdHj6BmyT2a', name: 'Ana Maria' }),
      changePassword: vi.fn().mockResolvedValue({}),
      signOutEverywhere: vi.fn().mockResolvedValue({ signedOut: true }),
    };
    setClient('$authClient', client);
  });

  async function loaded() {
    const p = useProfile();
    await p.load();
    return p;
  }

  it('starts from whoami: the person, their name and whether they can change a password', async () => {
    const p = await loaded();
    expect(p.person.value).toEqual({ id: 'V3StGXR8Z5jdHj6BmyT2a', name: 'Ana', email: 'ana@example.com', avatarUrl: '' });
    expect(p.name.value).toBe('Ana');
    expect(p.isPerson.value).toBe(true);
  });

  it('is not for keys or the basic login', async () => {
    client.whoami!.mockResolvedValue({ source: 'apikey', keyHint: 'abcd' });
    const p = await loaded();
    expect(p.isPerson.value).toBe(false);
    expect(p.person.value).toBeNull();
  });

  it('saves the name and shows it at once in the menu', async () => {
    const p = await loaded();
    p.name.value = '  Ana Maria ';
    await p.saveName();
    expect(client.updateProfile).toHaveBeenCalledWith({ name: 'Ana Maria' });
    expect(useAuthStore().me?.name).toBe('Ana Maria');
    expect(p.name.value).toBe('Ana Maria');
    expect(p.nameNotice.value).toBe('profile.name_saved');
    expect(p.nameError.value).toBeNull();
  });

  it('shows a translated error when the name is refused', async () => {
    setKnownKeys('apperror.USR003');
    client.updateProfile!.mockRejectedValueOnce(connectError('USR003', { max: '200' })).mockRejectedValueOnce(new Error('boom'));
    const p = await loaded();
    await p.saveName();
    expect(p.nameError.value).toEqual({ message: 'apperror.USR003|{"max":"200"}', reference: 'error.ref|{"id":"req-1"}' });
    expect(p.nameNotice.value).toBe('');
    await p.saveName();
    expect(p.nameError.value).toEqual({ message: 'error.update_profile' });
  });

  it('changes the password and clears the fields', async () => {
    const p = await loaded();
    p.currentPassword.value = 'old one';
    p.newPassword.value = 'a brand new passphrase';
    await p.changePassword();
    expect(client.changePassword).toHaveBeenCalledWith({ currentPassword: 'old one', newPassword: 'a brand new passphrase' });
    expect(p.currentPassword.value).toBe('');
    expect(p.newPassword.value).toBe('');
    expect(p.passwordNotice.value).toBe('profile.password_changed');
  });

  it('shows the server password errors: a wrong current one in profile words, and the policy', async () => {
    setKnownKeys('apperror.AUT001', 'apperror.AUT002_contains_name');
    client.changePassword!
      .mockRejectedValueOnce(connectError('AUT001'))
      .mockRejectedValueOnce(connectError('AUT002', { reason: 'contains_name', min: '12', max: '128' }));
    const p = await loaded();
    p.currentPassword.value = 'wrong';
    p.newPassword.value = 'ana maria rocks';
    await p.changePassword();
    expect(p.passwordError.value).toEqual({ message: 'profile.wrong_current_password', reference: 'error.ref|{"id":"req-1"}' });
    expect(p.currentPassword.value, 'nothing is cleared on failure').toBe('wrong');
    await p.changePassword();
    expect(p.passwordError.value?.message).toBe('apperror.AUT002_contains_name|{"min":"12","max":"128"}');
    expect(p.passwordNotice.value).toBe('');
  });

  it('asks before signing out everywhere, then goes to the sign-in page', async () => {
    runtimeConfig.public.apiUrl = '/app/api';
    const p = await loaded();
    p.askSignOut();
    expect(p.confirmingSignOut.value).toBe(true);
    expect(client.signOutEverywhere).not.toHaveBeenCalled();
    p.cancelSignOut();
    expect(p.confirmingSignOut.value).toBe(false);

    p.askSignOut();
    await p.signOutEverywhere();
    expect(client.signOutEverywhere).toHaveBeenCalledWith({});
    expect(navigateTo).toHaveBeenCalledWith('/app/login', { external: true });
    expect(p.confirmingSignOut.value).toBe(false);
  });

  it('stays put and shows an error when signing out everywhere fails', async () => {
    client.signOutEverywhere!.mockRejectedValue(new Error('boom'));
    const p = await loaded();
    p.askSignOut();
    await p.signOutEverywhere();
    expect(navigateTo).not.toHaveBeenCalled();
    expect(p.signOutError.value).toEqual({ message: 'error.sign_out_everywhere' });
  });

  it('does nothing while busy', async () => {
    const p = await loaded();
    p.busy.value = true;
    await p.saveName();
    await p.changePassword();
    await p.signOutEverywhere();
    expect(client.updateProfile).not.toHaveBeenCalled();
    expect(client.changePassword).not.toHaveBeenCalled();
    expect(client.signOutEverywhere).not.toHaveBeenCalled();
  });

  it('checks the name before sending and shows the server refusal under it', async () => {
    setKnownKeys('apperror.USR004');
    const p = await loaded();
    p.name.value = 'n'.repeat(201);
    await p.saveName();
    expect(client.updateProfile).not.toHaveBeenCalled();
    expect(p.nameFieldErrors.value).toEqual({ name: 'validation.too_long|{"max":"200"}' });

    client.updateProfile!.mockRejectedValueOnce(connectError('USR004', { max: '200' }));
    p.name.value = 'Ana';
    await p.saveName();
    expect(p.nameFieldErrors.value).toEqual({ name: 'apperror.USR004|{"max":"200"}' });
    expect(p.nameError.value).toBeNull();
  });

  it('checks the new password minimum before sending', async () => {
    const p = await loaded();
    p.currentPassword.value = 'old one';
    p.newPassword.value = 'short';
    await p.changePassword();
    expect(client.changePassword).not.toHaveBeenCalled();
    expect(p.passwordFieldErrors.value).toEqual({ new_password: 'validation.too_short|{"min":"12"}' });
    expect(p.passwordError.value).toBeNull();

    p.newPassword.value = 'a brand new passphrase';
    await p.changePassword();
    expect(client.changePassword).toHaveBeenCalled();
    expect(p.passwordFieldErrors.value).toEqual({});
  });
});
