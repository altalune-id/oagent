// @vitest-environment happy-dom
import { create } from '@bufbuild/protobuf';
import { flushPromises } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { WhoamiResponseSchema } from '#gen/auth/v1/auth_pb';
import { mountPage } from '#test/mount-page';
import { setClient, setRoute } from '#test/nuxt-globals';
import Orgs from '~/pages/orgs.vue';
import { useOrgsStore } from '~/stores/orgs';

beforeEach(() => {
  setActivePinia(createPinia());
  setRoute({ path: '/orgs' });
  setClient('$authClient', { whoami: vi.fn().mockResolvedValue(create(WhoamiResponseSchema, { permissions: [] })) });
});

describe('orgs page', () => {
  it('heads the page and lists each org as a dense row with its mono slug', async () => {
    const orgs = useOrgsStore();
    orgs.items = [{ id: 'O1', slug: 'acme', name: 'Acme', role: 'owner', permissions: [] }, { id: 'O2', slug: 'beta', name: 'Beta', role: 'member', permissions: [] }];
    orgs.loaded = true;
    const w = await mountPage(Orgs);
    expect(w.get('h1').classes()).toEqual(['text-xl', 'font-semibold', 'break-words']);
    const rows = w.findAll('[data-testid="org-list"] li');
    expect(rows.map(r => r.get('.font-mono').text())).toEqual(['/acme', '/beta']);
    expect(w.find('.card').exists()).toBe(false);
  });

  it('shows a load error with Retry instead of an empty list', async () => {
    const orgs = useOrgsStore();
    orgs.loaded = true;
    orgs.error = { message: 'down' } as never;
    const load = vi.spyOn(orgs, 'load').mockResolvedValue(undefined as never);
    const w = await mountPage(Orgs);
    expect(w.text()).not.toContain('org.none_title');
    await w.findAll('button').find(b => b.text() === 'common.retry')!.trigger('click');
    await flushPromises();
    expect(load).toHaveBeenCalled();
  });

  it('shows the shared empty state with Sign out when the person has no org and cannot create one', async () => {
    useOrgsStore().loaded = true;
    const w = await mountPage(Orgs);
    expect(w.get('h2').text()).toBe('org.none_title');
    expect(w.get('form button[type="submit"]').text()).toBe('auth.sign_out');
    expect(w.find('[data-testid="org-list"]').exists()).toBe(false);
  });
});
