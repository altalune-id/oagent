// @vitest-environment happy-dom
import { mount } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import SidebarNav from './SidebarNav.vue';

const items = [
  { key: 'profile', label: 'Profile', icon: 'lucide:user-round', active: true },
  { key: 'sessions', label: 'Sessions', active: false },
];

describe('sidebarNav', () => {
  it('is a named navigation landmark that marks only the current page', () => {
    const w = mount(SidebarNav, { props: { label: 'Account', items } });
    expect(w.get('nav').attributes('aria-label')).toBe('Account');
    const buttons = w.findAll('nav button');
    expect(buttons.map(b => b.attributes('aria-current'))).toEqual(['page', undefined]);
  });

  it('emits the picked item and drops the landmark name when there is no label', async () => {
    const w = mount(SidebarNav, { props: { items } });
    expect(w.get('nav').attributes('aria-label')).toBeUndefined();
    await w.findAll('nav button')[1]!.trigger('click');
    expect(w.emitted('select')).toEqual([['sessions']]);
  });

  it('shows the version in the footer', () => {
    expect(mount(SidebarNav, { props: { items, version: '1.2.3' } }).get('[data-sidebar-footer]').text()).toBe('OAgent v1.2.3');
  });
});
