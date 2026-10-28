import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { useOrgsStore } from '~/stores/orgs';
import { usePinnedStore } from '~/stores/pinned';
import { fakeStorage, setClient } from '../../../test/nuxt-globals';
import { useLanding } from './useLanding';

const acme = { id: 'V3StGXR8Z5jdHj6BmyT2a', slug: 'acme', name: 'Acme' };
const beta = { id: 'YeDxg2kMYznPJJG3wVmjj', slug: 'beta', name: 'Beta' };
const web = { id: 'x6MH9WPWjktMwCBKHy4pF', orgId: 'V3StGXR8Z5jdHj6BmyT2a', slug: 'web', name: 'Web' };
const api = { id: 'KdrF3t3FHtppBPmMgj4sy', orgId: 'V3StGXR8Z5jdHj6BmyT2a', slug: 'api', name: 'API' };

describe('useLanding', () => {
  let listProjects: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    setActivePinia(createPinia());
    vi.stubGlobal('localStorage', fakeStorage());
    listProjects = vi.fn().mockResolvedValue({ projects: [web, api] });
    setClient('$orgClient', {});
    setClient('$projectClient', { listProjects });
    setClient('$authClient', { whoami: vi.fn().mockResolvedValue({ source: 'local', userId: 'u1' }) });
  });

  function withOrgs(...list: typeof acme[]) {
    const orgs = useOrgsStore();
    list.forEach(org => orgs.add(org));
  }

  it('goes to the org list when there are no orgs', async () => {
    expect(await useLanding().resolve()).toBe('/orgs');
    expect(listProjects).not.toHaveBeenCalled();
  });

  it('goes to onboarding when there are no orgs and the caller may create one', async () => {
    setClient('$authClient', { whoami: vi.fn().mockResolvedValue({ source: 'oidc', userId: 'u1', orgCreation: true }) });
    expect(await useLanding().resolve()).toBe('/onboarding');
  });

  it('lands on the first project of the first org', async () => {
    withOrgs(acme, beta);
    expect(await useLanding().resolve()).toBe('/o/acme/projects/web');
    expect(listProjects).toHaveBeenCalledWith({ orgId: 'V3StGXR8Z5jdHj6BmyT2a' });
  });

  it('lands on the pinned org and project', async () => {
    withOrgs(beta, acme);
    usePinnedStore().setProject('acme', 'api');
    expect(await useLanding().resolve()).toBe('/o/acme/projects/api');
  });

  it('uses the requested org, falling back to the first org for an unknown slug', async () => {
    withOrgs(acme, beta);
    listProjects.mockResolvedValue({ projects: [] });
    expect(await useLanding().resolve('beta')).toBe('/o/beta');
    expect(await useLanding().resolve('nope')).toBe('/o/acme');
  });
});
