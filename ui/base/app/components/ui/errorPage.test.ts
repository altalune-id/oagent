// @vitest-environment happy-dom
import { mount } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import { errorKind } from './errorPage';
import ErrorPage from './ErrorPage.vue';

const labels = { title: 'Page not found', body: 'Gone.', homeLabel: 'Go to your organizations', retryLabel: 'Retry' };

describe('errorKind', () => {
  it.each([[404, 'not_found'], [403, 'forbidden'], [401, 'forbidden'], [500, 'failed'], [502, 'failed'], [undefined, 'failed']])('reads %s as %s', (code, kind) => {
    expect(errorKind(code)).toBe(kind);
  });
});

describe('errorPage', () => {
  it('has one h1 and only a way home when nothing can be retried', async () => {
    const w = mount(ErrorPage, { props: { kind: 'not_found', ...labels } });
    expect(w.findAll('h1')).toHaveLength(1);
    expect(w.get('h1').classes()).toEqual(['text-xl', 'font-semibold']);
    expect(w.get('p').classes()).toEqual(['text-ui', 'text-base-content/70']);
    expect(w.get('button').classes()).toEqual(expect.arrayContaining(['btn', 'btn-primary', 'btn-sm', 'text-ui']));
    expect(w.findAll('button').map(b => b.text())).toEqual(['Go to your organizations']);
    await w.get('button').trigger('click');
    expect(w.emitted('home')).toHaveLength(1);
  });

  it('offers Retry first on a failure', async () => {
    const w = mount(ErrorPage, { props: { kind: 'failed', ...labels } });
    expect(w.findAll('button').map(b => b.text())).toEqual(['Retry', 'Go to your organizations']);
    await w.get('button').trigger('click');
    expect(w.emitted('retry')).toHaveLength(1);
  });
});
