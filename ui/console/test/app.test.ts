// @vitest-environment happy-dom
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it } from 'vitest';

import { mountPage } from '#test/mount-page';
import { setLocale, useHead } from '#test/nuxt-globals';
import DefaultLayout from '~/layouts/default.vue';

beforeEach(() => {
  setActivePinia(createPinia());
});

describe('console shell', () => {
  it('sets <html lang> from the active locale and follows a switch', async () => {
    await mountPage(DefaultLayout, { global: { stubs: { AppNavbar: true, ToastHost: true, OrgCreateModal: true, ProjectCreateModal: true } } });
    const head = useHead.mock.calls[0]![0] as { htmlAttrs: { lang: { value: string } } };
    expect(head.htmlAttrs.lang.value).toBe('en-US');
    await setLocale('id-ID');
    expect(head.htmlAttrs.lang.value).toBe('id-ID');
  });
});
