// @vitest-environment happy-dom
import { mount } from '@vue/test-utils';
import { describe, expect, it, vi } from 'vitest';

import { setRoute } from '#test/nuxt-globals';
import ErrorView from '~/error.vue';

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
});
