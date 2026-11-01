// @vitest-environment happy-dom
import { mount, RouterLinkStub } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import SidebarNav from './SidebarNav.vue';

const groups = [
  { items: [{ key: 'overview', label: 'Overview', to: '/o/acme', icon: 'lucide:house', active: true }] },
  { label: 'Build', items: [{ key: 'agents', label: 'Agents', to: '/o/acme/agents', badge: 3 }] },
];

describe('sidebarNav', () => {
  it('is a named navigation landmark of links that marks only the current page', () => {
    const w = mount(SidebarNav, { props: { label: 'Project', groups } });
    expect(w.get('nav').attributes('aria-label')).toBe('Project');
    expect(w.findAllComponents(RouterLinkStub).map(l => l.props('to'))).toEqual(['/o/acme', '/o/acme/agents']);
    const links = w.findAll('nav a');
    expect(links.map(a => a.attributes('aria-current'))).toEqual(['page', undefined]);
    expect(links[0]!.classes()).toContain('menu-active');
  });

  it('titles a group only when it has a label, and shows item badges', () => {
    const w = mount(SidebarNav, { props: { groups } });
    expect(w.get('nav').attributes('aria-label')).toBeUndefined();
    expect(w.findAll('.menu-title').map(t => t.text())).toEqual(['Build']);
    expect(w.get('.badge').text()).toBe('3');
  });

  it('renders the top, back and footer slots only when given', () => {
    expect(mount(SidebarNav, { props: { groups } }).find('[data-sidebar-footer]').exists()).toBe(false);
    const w = mount(SidebarNav, { props: { groups }, slots: { top: 'Switcher', back: 'Back', footer: 'OAgent v1.2.3' } });
    expect(w.get('[data-sidebar-footer]').text()).toBe('OAgent v1.2.3');
    expect(w.text()).toContain('Switcher');
    expect(w.text()).toContain('Back');
  });
});
