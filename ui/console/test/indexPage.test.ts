// @vitest-environment happy-dom
import { flushPromises } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { mountPage } from '#test/mount-page';
import { setRoute } from '#test/nuxt-globals';
import Index from '~/pages/index.vue';
import { useOrgsStore } from '~/stores/orgs';

beforeEach(() => {
  setActivePinia(createPinia());
  setRoute({ path: '/' });
});

describe('landing page', () => {
  it('shows a load error with Retry when organizations cannot be read, and reloads them', async () => {
    const orgs = useOrgsStore();
    orgs.loaded = true;
    orgs.error = { message: 'down' } as never;
    const load = vi.spyOn(orgs, 'load').mockResolvedValue(undefined as never);
    const w = await mountPage(Index);
    await w.findAll('button').find(b => b.text() === 'common.retry')!.trigger('click');
    await flushPromises();
    expect(load).toHaveBeenCalled();
  });
});
