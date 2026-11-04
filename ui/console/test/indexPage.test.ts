// @vitest-environment happy-dom
import { create } from '@bufbuild/protobuf';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { WhoamiResponseSchema } from '#gen/auth/v1/auth_pb';

import { mountPage } from '#test/mount-page';
import { navigateTo, setClient, setRoute } from '#test/nuxt-globals';
import Index from '~/pages/index.vue';
import { useOrgsStore } from '~/stores/orgs';

beforeEach(() => {
  setActivePinia(createPinia());
  setRoute({ path: '/' });
  setClient('$authClient', { whoami: vi.fn().mockResolvedValue(create(WhoamiResponseSchema, { orgCreation: true, permissions: [] })) });
});

describe('landing page', () => {
  it('sends a failed organization read to the orgs page, which shows the header and Retry', async () => {
    const orgs = useOrgsStore();
    orgs.loaded = true;
    orgs.error = { message: 'down' } as never;
    await mountPage(Index);
    expect(navigateTo).toHaveBeenCalledWith('/orgs', { replace: true });
  });
});
