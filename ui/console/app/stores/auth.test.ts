import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { deferred, setClient } from '#test/nuxt-globals';
import { useAuthStore } from './auth';

describe('useAuthStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('loads the signed-in user once', async () => {
    const pending = deferred<{ source: string }>();
    const whoami = vi.fn(() => pending.promise);
    setClient('$authClient', { whoami });
    const auth = useAuthStore();
    const first = auth.load();
    const second = auth.load();
    pending.resolve({ source: 'local' });
    await Promise.all([first, second]);
    expect(whoami).toHaveBeenCalledTimes(1);
    expect(auth.me).toEqual({ source: 'local' });
    expect(auth.loaded).toBe(true);
  });

  it('keeps me null when whoami fails', async () => {
    setClient('$authClient', { whoami: vi.fn().mockRejectedValue(new Error('401')) });
    const auth = useAuthStore();
    await auth.load();
    expect(auth.me).toBeNull();
    expect(auth.loaded).toBe(true);
  });

  it('asks again after a failure, but not after a success', async () => {
    const whoami = vi.fn().mockRejectedValueOnce(new Error('offline')).mockResolvedValue({ source: 'local' });
    setClient('$authClient', { whoami });
    const auth = useAuthStore();
    await auth.load();
    expect(auth.me).toBeNull();
    await auth.load();
    expect(auth.me).toEqual({ source: 'local' });
    await auth.load();
    expect(whoami).toHaveBeenCalledTimes(2);
  });
});

describe('useAuthStore.setActiveOrg', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('switches the active org and keeps the returned whoami', async () => {
    const setActiveOrg = vi.fn().mockResolvedValue({ me: { source: 'local', activeOrgSlug: 'beta' } });
    setClient('$authClient', { whoami: vi.fn().mockResolvedValue({ source: 'local', activeOrgSlug: 'acme' }), setActiveOrg });
    const auth = useAuthStore();
    await auth.load();
    await auth.setActiveOrg('betaOrgPublicId000001');
    expect(setActiveOrg).toHaveBeenCalledWith({ orgId: 'betaOrgPublicId000001' });
    expect(auth.me).toEqual({ source: 'local', activeOrgSlug: 'beta' });
  });

  it('keeps the old whoami when the switch fails', async () => {
    setClient('$authClient', {
      whoami: vi.fn().mockResolvedValue({ source: 'local', activeOrgSlug: 'acme' }),
      setActiveOrg: vi.fn().mockRejectedValue(new Error('not found')),
    });
    const auth = useAuthStore();
    await auth.load();
    await expect(auth.setActiveOrg('x')).rejects.toThrow('not found');
    expect(auth.me).toEqual({ source: 'local', activeOrgSlug: 'acme' });
  });
});
