// @vitest-environment happy-dom
import { mount } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import AppBadge from './AppBadge.vue';
import StatusLabel from './StatusLabel.vue';

describe('appBadge', () => {
  it('is a small badge with an optional hidden icon, xs and outline on request', () => {
    const w = mount(AppBadge, { props: { label: 'Needs approval', icon: 'lucide:hand' } });
    expect(w.classes()).toEqual(['badge', 'badge-sm']);
    expect(w.text()).toBe('Needs approval');
    expect(w.get('[data-icon="lucide:hand"]').attributes('aria-hidden')).toBe('true');
    expect(mount(AppBadge, { props: { label: 'Draft changes', outline: true } }).classes()).toEqual(['badge', 'badge-sm', 'badge-outline']);
    expect(mount(AppBadge, { props: { label: 'Production', size: 'xs' } }).classes()).toEqual(['badge', 'badge-xs']);
  });
});

describe('statusLabel', () => {
  it('never shows status by colour alone: a hidden dot, a label and an optional muted detail', () => {
    const w = mount(StatusLabel, { props: { tone: 'warning', label: 'Waiting for approval', detail: 'refund over $200' } });
    const dot = w.get('.status');
    expect(dot.classes()).toEqual(['status', 'status-warning']);
    expect(dot.attributes('aria-hidden')).toBe('true');
    expect(w.get('.font-medium').text()).toBe('Waiting for approval');
    expect(w.text()).toContain('refund over $200');
    expect(mount(StatusLabel, { props: { tone: 'error', label: 'Failed' } }).find('.text-base-content\\/70').exists()).toBe(false);
  });
});
