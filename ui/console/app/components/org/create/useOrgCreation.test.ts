import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it } from 'vitest';

import { useAuthStore } from '~/stores/auth';
import { useOrgsStore } from '~/stores/orgs';
import { useOrgCreation } from './useOrgCreation';

const acme = { id: 'V3StGXR8Z5jdHj6BmyT2a', slug: 'acme', name: 'Acme' };

describe('useOrgCreation', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  function given(orgCreation: boolean, role?: string) {
    useAuthStore().me = { orgCreation } as never;
    useOrgsStore().items = role ? [{ ...acme, role }] : [];
    return useOrgCreation();
  }

  it('offers creation where the deployment allows it and the person owns no org', () => {
    expect(given(true).canCreate.value).toBe(true);
    expect(given(true, 'admin').canCreate.value).toBe(true);
  });

  it('withholds it from someone who already owns an org, as the server would answer ORG006', () => {
    const c = given(true, 'owner');
    expect(c.allowed.value).toBe(true);
    expect(c.ownsOne.value).toBe(true);
    expect(c.canCreate.value).toBe(false);
  });

  it('never offers it where the deployment does not', () => {
    expect(given(false).canCreate.value).toBe(false);
    expect(given(false, 'owner').ownsOne.value).toBe(true);
  });
});
