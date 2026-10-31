// @vitest-environment happy-dom
import { mount } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import LoadingStatus from './LoadingStatus.vue';

describe('loadingStatus', () => {
  it('is a status with a spoken label and a hidden spinner', () => {
    const w = mount(LoadingStatus, { props: { label: 'Loading…' } });
    expect(w.attributes('role')).toBe('status');
    expect(w.text()).toBe('Loading…');
    expect(w.get('.loading').attributes('aria-hidden')).toBe('true');
  });
});
