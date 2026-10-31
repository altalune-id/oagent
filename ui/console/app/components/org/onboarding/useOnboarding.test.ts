import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { connectError, fakeStorage, navigateTo, setClient, setKnownKeys } from '#test/nuxt-globals';
import { useOrgsStore } from '~/stores/orgs';
import { useOrgCreation } from '../create/useOrgCreation';
import { useOnboarding } from './useOnboarding';

const org = { id: 'V3StGXR8Z5jdHj6BmyT2a', slug: 'acme', name: 'Acme' };
const project = { id: 'x6MH9WPWjktMwCBKHy4pF', orgId: org.id, slug: 'default', name: 'Default' };

describe('useOnboarding', () => {
  let listProjects: ReturnType<typeof vi.fn>;
  let createOwnOrg: ReturnType<typeof vi.fn>;
  let createOrg: ReturnType<typeof vi.fn>;
  let setActiveOrg: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    setActivePinia(createPinia());
    vi.stubGlobal('localStorage', fakeStorage());
    createOwnOrg = vi.fn().mockResolvedValue({ org });
    createOrg = vi.fn();
    setActiveOrg = vi.fn().mockResolvedValue({});
    setClient('$authClient', { whoami: vi.fn().mockResolvedValue({ source: 'local', userId: 'u1', orgCreation: true }), setActiveOrg, createOwnOrg });
    setClient('$orgClient', { createOrg, listOrgs: vi.fn().mockResolvedValue({ orgs: [org] }) });
    listProjects = vi.fn().mockResolvedValue({ projects: [project] });
    setClient('$projectClient', { listProjects });
  });

  async function onboard() {
    const form = useOnboarding();
    form.name.value = 'Acme';
    await form.submit();
  }

  it('creates the caller\'s own org and lands on the default project the server made with it', async () => {
    await onboard();
    expect(createOwnOrg).toHaveBeenCalledWith({ name: 'Acme', slug: '' });
    expect(createOrg).not.toHaveBeenCalled();
    expect(useOrgsStore().bySlug('acme')?.id).toBe(org.id);
    expect(setActiveOrg).toHaveBeenCalledWith({ orgId: org.id });
    expect(listProjects).toHaveBeenCalledWith({ orgId: org.id });
    expect(navigateTo).toHaveBeenCalledWith('/o/acme/projects/default');
  });

  it('stops offering New organization once onboarding created the caller\'s own org', async () => {
    const orgs = useOrgsStore();
    orgs.loaded = true;
    const creation = useOrgCreation();
    const form = useOnboarding();
    form.name.value = 'Acme';
    await form.submit();
    expect(orgs.bySlug('acme')?.role).toBe('owner');
    expect(creation.ownsOne.value).toBe(true);
  });

  it('takes someone who already owns an org there', async () => {
    setKnownKeys('apperror.ORG006');
    createOwnOrg.mockRejectedValue(connectError('ORG006'));
    const form = useOnboarding();
    form.name.value = 'Other';
    await form.submit();
    expect(form.error.value?.message).toBe('apperror.ORG006');
    expect(navigateTo).toHaveBeenCalledWith('/o/acme/projects/default', { replace: true });
  });

  it('stays on the form for any other refusal', async () => {
    createOwnOrg.mockRejectedValue(connectError('ORG004'));
    const form = useOnboarding();
    form.name.value = 'Acme';
    await form.submit();
    expect(navigateTo).not.toHaveBeenCalled();
  });

  it('lands on the org when it has no project', async () => {
    listProjects.mockResolvedValue({ projects: [] });
    await onboard();
    expect(navigateTo).toHaveBeenCalledWith('/o/acme');
  });
});
