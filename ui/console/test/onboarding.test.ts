// @vitest-environment happy-dom
import { create } from '@bufbuild/protobuf';
import { flushPromises } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { WhoamiResponseSchema } from '#gen/auth/v1/auth_pb';
import { mountPage } from '#test/mount-page';
import { setClient, setRoute } from '#test/nuxt-globals';
import Onboarding from '~/pages/onboarding.vue';
import { useOrgsStore } from '~/stores/orgs';

beforeEach(() => {
  setActivePinia(createPinia());
  setRoute({ path: '/onboarding' });
  setClient('$authClient', { whoami: vi.fn().mockResolvedValue(create(WhoamiResponseSchema, { orgCreation: true, permissions: [] })) });
});

describe('onboarding', () => {
  it('is one bordered box with the design title and the org fields, not a daisyUI card', async () => {
    useOrgsStore().loaded = true;
    const w = await mountPage(Onboarding);
    expect(w.find('.card').exists()).toBe(false);
    expect(w.get('h1').classes()).toEqual(['text-xl', 'font-semibold']);
    expect(w.get('form').classes()).toEqual(expect.arrayContaining(['rounded-box', 'border', 'border-base-300']));
    expect(w.findAll('form input').length).toBe(2);
  });

  it('shows a load error with Retry when organizations cannot be read', async () => {
    const orgs = useOrgsStore();
    orgs.loaded = true;
    orgs.error = { message: 'down' } as never;
    const load = vi.spyOn(orgs, 'load').mockResolvedValue(undefined as never);
    const w = await mountPage(Onboarding);
    expect(w.find('form').exists()).toBe(false);
    await w.findAll('button').find(b => b.text() === 'common.retry')!.trigger('click');
    await flushPromises();
    expect(load).toHaveBeenCalled();
  });
});
