import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { fakeStorage } from '../../test/nuxt-globals';
import { usePinnedStore } from './pinned';

describe('usePinnedStore', () => {
  let storage: ReturnType<typeof fakeStorage>;

  beforeEach(() => {
    setActivePinia(createPinia());
    storage = fakeStorage();
    vi.stubGlobal('localStorage', storage);
  });

  function saved() {
    return JSON.parse(storage.data.get('oagent_pinned') ?? 'null');
  }

  it('starts empty with nothing saved', () => {
    const pinned = usePinnedStore();
    expect(pinned.orgSlug).toBe('');
    expect(pinned.projectFor('acme')).toBe('');
  });

  it('reads the saved pins', () => {
    storage.data.set('oagent_pinned', JSON.stringify({ org: 'acme', projects: { acme: 'web' } }));
    const pinned = usePinnedStore();
    expect(pinned.orgSlug).toBe('acme');
    expect(pinned.projectFor('acme')).toBe('web');
  });

  it('ignores saved pins without fields', () => {
    storage.data.set('oagent_pinned', JSON.stringify({}));
    const pinned = usePinnedStore();
    expect(pinned.orgSlug).toBe('');
  });

  it('ignores corrupt saved pins', () => {
    storage.data.set('oagent_pinned', '{not json');
    const pinned = usePinnedStore();
    expect(pinned.orgSlug).toBe('');
    expect(pinned.projectFor('acme')).toBe('');
  });

  it('pins and unpins, saving each change', () => {
    const pinned = usePinnedStore();
    pinned.setOrg('acme');
    expect(saved()).toEqual({ org: 'acme', projects: {} });
    pinned.setProject('beta', 'api');
    expect(pinned.orgSlug).toBe('beta');
    expect(pinned.projectFor('beta')).toBe('api');
    expect(saved()).toEqual({ org: 'beta', projects: { beta: 'api' } });
    pinned.unpinProject('beta');
    expect(pinned.projectFor('beta')).toBe('');
    pinned.unpinOrg();
    expect(pinned.orgSlug).toBe('');
    expect(saved()).toEqual({ org: '', projects: {} });
    expect(storage.setItem).toHaveBeenCalledTimes(4);
  });
});
