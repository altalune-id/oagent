import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { fakeStorage } from '#test/nuxt-globals';
import { useRecentStore } from './recent';

describe('useRecentStore', () => {
  let storage: ReturnType<typeof fakeStorage>;

  beforeEach(() => {
    setActivePinia(createPinia());
    storage = fakeStorage();
    vi.stubGlobal('localStorage', storage);
  });

  const saved = () => JSON.parse(storage.data.get('oagent_recent') ?? 'null');

  it('moves each visit to the front, per org for projects, and keeps it', () => {
    const recent = useRecentStore();
    recent.visit('acme', 'support');
    recent.visit('north');
    recent.visit('acme', 'invoices');
    expect(recent.orgs).toEqual(['acme', 'north']);
    expect(recent.lastOrg()).toBe('acme');
    expect(recent.lastProject('acme')).toBe('invoices');
    expect(saved()).toEqual({ orgs: ['acme', 'north'], projects: { acme: ['invoices', 'support'] } });
  });

  it('keeps 20 orgs and drops the projects of an org that fell off', () => {
    const recent = useRecentStore();
    recent.visit('org-0', 'p');
    for (let i = 1; i <= 20; i++) {
      recent.visit(`org-${i}`);
    }
    expect(recent.orgs).toHaveLength(20);
    expect(recent.orgs).not.toContain('org-0');
    expect(recent.projects).toEqual({});
  });

  it('orders items by recency, unvisited last in their own order', () => {
    const recent = useRecentStore();
    recent.visit('b');
    recent.visit('c');
    expect(recent.byRecency(['a', 'b', 'c', 'd'], s => s, recent.orgs)).toEqual(['c', 'b', 'a', 'd']);
  });

  it('reads only the saved shape: a non-array list counts as empty and projects follow the kept orgs', () => {
    storage.data.set('oagent_recent', JSON.stringify({ orgs: 'acme', projects: { acme: ['x'] } }));
    expect(useRecentStore().orgs).toEqual([]);
    setActivePinia(createPinia());
    storage.data.set('oagent_recent', JSON.stringify({ orgs: ['acme', 7, ''], projects: { acme: 'x', gone: ['y'] } }));
    const recent = useRecentStore();
    expect(recent.orgs).toEqual(['acme']);
    expect(recent.projects).toEqual({ acme: [] });
    setActivePinia(createPinia());
    storage.data.set('oagent_recent', 'null');
    expect(useRecentStore().orgs).toEqual([]);
  });

  it('migrates the old pins once and removes the old key', () => {
    storage.data.set('oagent_pinned', JSON.stringify({ org: 'acme', projects: { acme: 'support', north: 'ops' } }));
    const recent = useRecentStore();
    expect(recent.orgs).toEqual(['acme', 'north']);
    expect(recent.lastProject('north')).toBe('ops');
    expect(storage.data.has('oagent_pinned')).toBe(false);
    expect(saved().orgs).toEqual(['acme', 'north']);
  });

  it('removes a corrupt old pin key too', () => {
    storage.data.set('oagent_pinned', '{not json');
    expect(useRecentStore().orgs).toEqual([]);
    expect(storage.data.has('oagent_pinned')).toBe(false);
  });

  it('keeps working when the browser refuses to store anything', () => {
    storage.setItem.mockImplementation(() => {
      throw new Error('QuotaExceededError');
    });
    storage.removeItem.mockImplementation(() => {
      throw new Error('SecurityError');
    });
    const recent = useRecentStore();
    expect(() => recent.visit('acme', 'support')).not.toThrow();
    expect(recent.lastProject('acme')).toBe('support');
  });
});
