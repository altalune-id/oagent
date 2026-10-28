import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { useOrgsStore } from '~/stores/orgs';
import { connectError, fakeStorage, navigateTo, setClient, setKnownKeys } from '../../../../test/nuxt-globals';
import { useOnboarding } from './useOnboarding';

const org = { id: 'V3StGXR8Z5jdHj6BmyT2a', slug: 'acme', name: 'Acme' };
const project = { id: 'x6MH9WPWjktMwCBKHy4pF', orgId: org.id, slug: 'default', name: 'Default' };

describe('useOnboarding', () => {
  let listProjects: ReturnType<typeof vi.fn>;
  let createOwnOrg: ReturnType<typeof vi.fn>;
  let createOrg: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    setActivePinia(createPinia());
    vi.stubGlobal('localStorage', fakeStorage());
    createOwnOrg = vi.fn().mockResolvedValue({ org });
    createOrg = vi.fn();
    setClient('$authClient', { whoami: vi.fn().mockResolvedValue({ source: 'local', orgCreation: true }), createOwnOrg });
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
    expect(listProjects).toHaveBeenCalledWith({ orgId: org.id });
    expect(navigateTo).toHaveBeenCalledWith('/o/acme/projects/default');
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
