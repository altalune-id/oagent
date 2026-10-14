import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { connectError, deferred, setClient, setKnownKeys } from '../../test/nuxt-globals';
import { useOrgsStore } from './orgs';

const acme = { id: 'V3StGXR8Z5jdHj6BmyT2a', slug: 'acme', name: 'Acme' };

describe('useOrgsStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('loads the orgs', async () => {
    const listOrgs = vi.fn().mockResolvedValue({ orgs: [{ ...acme, extra: 'ignored' }] });
    setClient('$orgClient', { listOrgs });
    const orgs = useOrgsStore();
    await orgs.load();
    expect(orgs.items).toEqual([acme]);
    expect(orgs.loaded).toBe(true);
    expect(orgs.loading).toBe(false);
    expect(orgs.error).toBeNull();
    expect(orgs.bySlug('acme')).toEqual(acme);
    expect(orgs.bySlug('nope')).toBeUndefined();
  });

  it('treats a missing list as empty', async () => {
    setClient('$orgClient', { listOrgs: vi.fn().mockResolvedValue({}) });
    const orgs = useOrgsStore();
    await orgs.load();
    expect(orgs.items).toEqual([]);
    expect(orgs.loaded).toBe(true);
  });

  it('shows a Connect error with its reference', async () => {
    setKnownKeys('apperror.GEN900');
    setClient('$orgClient', { listOrgs: vi.fn().mockRejectedValue(connectError('GEN900', {}, 'req-7')) });
    const orgs = useOrgsStore();
    await orgs.load();
    expect(orgs.error).toEqual({ message: 'apperror.GEN900', reference: 'error.ref|{"id":"req-7"}' });
    expect(orgs.loaded).toBe(false);
    expect(orgs.loading).toBe(false);
  });

  it('shows the fallback for a plain error', async () => {
    setClient('$orgClient', { listOrgs: vi.fn().mockRejectedValue(new Error('boom')) });
    const orgs = useOrgsStore();
    await orgs.load();
    expect(orgs.error).toEqual({ message: 'error.load_orgs' });
  });

  it('shares one request between concurrent ensureLoaded calls, then skips once loaded', async () => {
    const pending = deferred<{ orgs: typeof acme[] }>();
    const listOrgs = vi.fn(() => pending.promise);
    setClient('$orgClient', { listOrgs });
    const orgs = useOrgsStore();
    const first = orgs.ensureLoaded();
    const second = orgs.ensureLoaded();
    expect(listOrgs).toHaveBeenCalledTimes(1);
    expect(orgs.loading).toBe(true);
    pending.resolve({ orgs: [acme] });
    await Promise.all([first, second]);
    await orgs.ensureLoaded();
    expect(listOrgs).toHaveBeenCalledTimes(1);
    expect(orgs.items).toEqual([acme]);
  });

  it('retries ensureLoaded after a failure', async () => {
    const listOrgs = vi.fn().mockRejectedValueOnce(new Error('boom')).mockResolvedValueOnce({ orgs: [acme] });
    setClient('$orgClient', { listOrgs });
    const orgs = useOrgsStore();
    await orgs.ensureLoaded();
    expect(orgs.error).toEqual({ message: 'error.load_orgs' });
    await orgs.ensureLoaded();
    expect(orgs.error).toBeNull();
    expect(listOrgs).toHaveBeenCalledTimes(2);
  });

  it('adds an org once and updates it', () => {
    setClient('$orgClient', {});
    const orgs = useOrgsStore();
    orgs.add(acme);
    orgs.add({ ...acme, id: 'dup' });
    expect(orgs.items).toEqual([acme]);
    orgs.add({ ...acme, id: 'YeDxg2kMYznPJJG3wVmjj', slug: 'beta' });
    orgs.update('V3StGXR8Z5jdHj6BmyT2a', { name: 'Acme Inc', slug: 'acme-inc' });
    expect(orgs.items.map(o => [o.id, o.slug, o.name])).toEqual([['V3StGXR8Z5jdHj6BmyT2a', 'acme-inc', 'Acme Inc'], ['YeDxg2kMYznPJJG3wVmjj', 'beta', 'Acme']]);
  });
});
