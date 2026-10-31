import { Code, ConnectError } from '@connectrpc/connect';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { setClient } from '#test/nuxt-globals';
import { useOrgsStore } from '~/stores/orgs';
import { useActiveOrg } from './useActiveOrg';

function setup(me: Record<string, unknown>) {
  const setActiveOrg = vi.fn().mockResolvedValue({ me: { ...me, activeOrgSlug: 'beta' } });
  setClient('$authClient', { whoami: vi.fn().mockResolvedValue(me), setActiveOrg });
  setClient('$orgClient', {
    listOrgs: vi.fn().mockResolvedValue({ orgs: [{ id: 'acmeId', slug: 'acme', name: 'Acme' }, { id: 'betaId', slug: 'beta', name: 'Beta' }] }),
  });
  return setActiveOrg;
}

describe('useActiveOrg', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('switches a person to the org they open', async () => {
    const setActiveOrg = setup({ source: 'local', userId: 'u1', activeOrgSlug: 'acme' });
    await useActiveOrg().enter('beta');
    expect(setActiveOrg).toHaveBeenCalledWith({ orgId: 'betaId' });
  });

  it('switches someone who signed in through the OIDC provider too', async () => {
    const setActiveOrg = setup({ source: 'oidc', userId: 'u1', activeOrgSlug: 'acme' });
    await useActiveOrg().enter('beta');
    expect(setActiveOrg).toHaveBeenCalledWith({ orgId: 'betaId' });
  });

  it('does nothing when the org is already active', async () => {
    const setActiveOrg = setup({ source: 'local', userId: 'u1', activeOrgSlug: 'beta' });
    await useActiveOrg().enter('beta');
    expect(setActiveOrg).not.toHaveBeenCalled();
  });

  it('does nothing for an org the person does not have (the page answers not found)', async () => {
    const setActiveOrg = setup({ source: 'local', userId: 'u1', activeOrgSlug: 'acme' });
    await useActiveOrg().enter('foreign');
    expect(setActiveOrg).not.toHaveBeenCalled();
  });

  it('never switches a key or basic auth, which reach orgs without it', async () => {
    for (const me of [{ source: 'apikey' }, { source: 'basic' }]) {
      setActivePinia(createPinia());
      const setActiveOrg = setup(me);
      await useActiveOrg().enter('beta');
      expect(setActiveOrg).not.toHaveBeenCalled();
    }
  });

  it('switches an operator to an org they belong to, so project pages there load', async () => {
    const setActiveOrg = setup({ source: 'local', userId: 'u1', operator: true, activeOrgSlug: 'acme' });
    await useActiveOrg().enter('beta');
    expect(setActiveOrg).toHaveBeenCalledWith({ orgId: 'betaId' });
  });

  it('keeps an operator in an org they do not belong to when the switch is refused', async () => {
    setup({ source: 'local', userId: 'u1', operator: true, activeOrgSlug: 'acme' });
    const setActiveOrg = vi.fn().mockRejectedValue(new ConnectError('not found', Code.NotFound));
    setClient('$authClient', { whoami: vi.fn().mockResolvedValue({ source: 'local', userId: 'u1', operator: true, activeOrgSlug: 'acme' }), setActiveOrg });
    await expect(useActiveOrg().enter('beta')).resolves.toBeUndefined();
    expect(setActiveOrg).toHaveBeenCalledWith({ orgId: 'betaId' });
  });

  it('does nothing with nobody signed in', async () => {
    setClient('$authClient', { whoami: vi.fn().mockRejectedValue(new Error('401')), setActiveOrg: vi.fn() });
    setClient('$orgClient', { listOrgs: vi.fn().mockResolvedValue({ orgs: [] }) });
    await expect(useActiveOrg().enter('beta')).resolves.toBeUndefined();
  });

  it('lets a failed switch surface', async () => {
    setup({ source: 'local', userId: 'u1', activeOrgSlug: 'acme' });
    setClient('$authClient', {
      whoami: vi.fn().mockResolvedValue({ source: 'local', userId: 'u1', activeOrgSlug: 'acme' }),
      setActiveOrg: vi.fn().mockRejectedValue(new Error('boom')),
    });
    await expect(useActiveOrg().enter('beta')).rejects.toThrow('boom');
  });

  it('reloads the orgs once when the org is not cached yet, then switches', async () => {
    const setActiveOrg = setup({ source: 'local', userId: 'u1', activeOrgSlug: 'acme' });
    const listOrgs = vi.fn()
      .mockResolvedValueOnce({ orgs: [{ id: 'acmeId', slug: 'acme', name: 'Acme' }] })
      .mockResolvedValueOnce({ orgs: [{ id: 'acmeId', slug: 'acme', name: 'Acme' }, { id: 'newId', slug: 'new', name: 'New' }] });
    setClient('$orgClient', { listOrgs });
    await useActiveOrg().enter('new');
    expect(listOrgs).toHaveBeenCalledTimes(2);
    expect(setActiveOrg).toHaveBeenCalledWith({ orgId: 'newId' });
  });

  it('leaves an org still unknown after one reload to the page, which shows not found', async () => {
    const setActiveOrg = setup({ source: 'local', userId: 'u1', activeOrgSlug: 'acme' });
    const listOrgs = vi.fn().mockResolvedValue({ orgs: [{ id: 'acmeId', slug: 'acme', name: 'Acme' }] });
    setClient('$orgClient', { listOrgs });
    await expect(useActiveOrg().enter('gone')).resolves.toBeUndefined();
    expect(listOrgs).toHaveBeenCalledTimes(2);
    expect(setActiveOrg).not.toHaveBeenCalled();
  });

  it('reloads the orgs once when the switch answers not found, and lets the page show it', async () => {
    setup({ source: 'local', userId: 'u1', activeOrgSlug: 'acme' });
    const listOrgs = vi.fn()
      .mockResolvedValueOnce({ orgs: [{ id: 'acmeId', slug: 'acme', name: 'Acme' }, { id: 'betaId', slug: 'beta', name: 'Beta' }] })
      .mockResolvedValueOnce({ orgs: [{ id: 'acmeId', slug: 'acme', name: 'Acme' }] });
    const setActiveOrg = vi.fn().mockRejectedValue(new ConnectError('gone', Code.NotFound));
    setClient('$orgClient', { listOrgs });
    setClient('$authClient', { whoami: vi.fn().mockResolvedValue({ source: 'local', userId: 'u1', activeOrgSlug: 'acme' }), setActiveOrg });
    await expect(useActiveOrg().enter('beta')).resolves.toBeUndefined();
    expect(setActiveOrg).toHaveBeenCalledTimes(1);
    expect(listOrgs).toHaveBeenCalledTimes(2);
    expect(useOrgsStore().bySlug('beta')).toBeUndefined();
  });
});
