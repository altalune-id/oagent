// @vitest-environment happy-dom
import { flushPromises, mount } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { describe, expect, it, vi } from 'vitest';

import { setRoute } from '#test/nuxt-globals';
import ErrorView from '~/error.vue';
import DefaultLayout from '~/layouts/default.vue';

function render(error: { statusCode: number, statusMessage?: string }) {
  const clearError = vi.fn();
  vi.stubGlobal('clearError', clearError);
  const w = mount(ErrorView, { props: { error: error as never }, global: { stubs: { NuxtLayout: { template: '<div data-testid="layout"><slot /></div>' } } } });
  return { w, clearError };
}

describe('console error page', () => {
  it('keeps the app layout, names a missing org and goes back to the orgs', async () => {
    const { w, clearError } = render({ statusCode: 404, statusMessage: 'Organization not found' });
    expect(w.find('[data-testid="layout"] h1').text()).toBe('error_page.not_found_title');
    expect(w.text()).toContain('Organization not found');
    await w.get('button').trigger('click');
    expect(clearError).toHaveBeenCalledWith({ redirect: '/orgs' });
  });

  it('retries a failure on the same address, with the generic line instead of the raw message', async () => {
    setRoute({ path: '/o/acme', params: { org: 'acme' } });
    const { w, clearError } = render({ statusCode: 502, statusMessage: 'upstream exploded' });
    expect(w.text()).toContain('error_page.failed_body');
    expect(w.text()).not.toContain('upstream exploded');
    await w.findAll('button').find(b => b.text().includes('common.retry'))!.trigger('click');
    expect(clearError).toHaveBeenCalledWith({ redirect: '/o/acme' });
  });

  it('paints the theme the viewer picked, since Nuxt renders it instead of app.vue', async () => {
    setActivePinia(createPinia());
    localStorage.setItem('oagent_theme', JSON.stringify({ family: 'retro', mode: 'light' }));
    document.documentElement.removeAttribute('data-theme');
    const stubs = { AppNavbar: true, ToastHost: true, OrgCreateModal: true, ProjectCreateModal: true };
    mount(ErrorView, { props: { error: { statusCode: 502 } as never }, global: { stubs: { ...stubs, NuxtLayout: DefaultLayout } } });
    await flushPromises();
    expect(document.documentElement.getAttribute('data-theme')).toBe('retro-light');
  });
});
