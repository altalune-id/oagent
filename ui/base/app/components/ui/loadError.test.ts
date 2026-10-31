// @vitest-environment happy-dom
import { mount } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import ErrorText from './ErrorText.vue';
import LoadError from './LoadError.vue';

const error = { message: 'Couldn\'t load your sessions.', reference: 'Ref: req-1' };

describe('loadError', () => {
  it('shows nothing without an error', () => {
    expect(mount(LoadError, { props: { error: null } }).find('button').exists()).toBe(false);
  });

  it('shows the error once as the alert, with a Retry that asks the page to load again', async () => {
    const w = mount(LoadError, { props: { error, retryLabel: 'Retry' } });
    expect(w.findAll('[role="alert"]')).toHaveLength(1);
    expect(w.text()).toContain('Couldn\'t load your sessions.');
    expect(w.text()).toContain('Ref: req-1');
    await w.get('button').trigger('click');
    expect(w.emitted('retry')).toHaveLength(1);
  });

  it('disables Retry and shows a hidden spinner while it retries', () => {
    const w = mount(LoadError, { props: { error, busy: true } });
    expect(w.get('button').attributes('disabled')).toBeDefined();
    expect(w.get('.loading').attributes('aria-hidden')).toBe('true');
  });
});

describe('errorText', () => {
  it('announces itself as an alert, and renders nothing without an error', () => {
    expect(mount(ErrorText, { props: { error } }).get('p').attributes('role')).toBe('alert');
    expect(mount(ErrorText, { props: { error: null } }).find('p').exists()).toBe(false);
  });
});
