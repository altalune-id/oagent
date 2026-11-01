// @vitest-environment happy-dom
import { mount } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import { clearError, setRoute } from '#test/nuxt-globals';
import ErrorPage from '~/error.vue';

function render(statusCode: number, statusMessage = '') {
  return mount(ErrorPage, { props: { error: { statusCode, statusMessage } }, global: { stubs: { NuxtLayout: { template: '<div><slot /></div>' } } } });
}

describe('error page', () => {
  it('keeps the app layout and sends a missing page home', async () => {
    const w = render(404, 'Organization not found');
    expect(w.get('h1').text()).toBe('error.page_not_found');
    expect(w.text()).toContain('Organization not found');
    expect(w.findAll('button')).toHaveLength(1);
    await w.get('button').trigger('click');
    expect(clearError).toHaveBeenCalledWith({ redirect: '/orgs' });
  });

  it('explains a refusal without a retry', () => {
    const w = render(403);
    expect(w.get('h1').text()).toBe('error.page_forbidden');
    expect(w.findAll('button')).toHaveLength(1);
    expect(w.find('p.mt-4').exists()).toBe(false);
  });

  it('offers a retry of the same page when loading failed', async () => {
    setRoute({ path: '/o/acme' });
    const w = render(502);
    expect(w.get('h1').text()).toBe('error.page_failed');
    await w.findAll('button')[1]!.trigger('click');
    expect(clearError).toHaveBeenCalledWith({ redirect: '/o/acme' });
  });
});
