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

  it('draws the design menu: dense small menu, muted group titles, a bordered footer', () => {
    const w = mount(SidebarNav, { props: { groups }, slots: { footer: 'Sam Rivera' } });
    expect(w.get('aside').classes()).toEqual(['flex', 'h-full', 'min-h-full', 'w-60', 'shrink-0', 'flex-col', 'border-e', 'border-base-300', 'bg-base-200']);
    expect(w.get('ul').classes()).toEqual(['menu', 'menu-sm', 'w-full', 'text-ui', 'py-0']);
    expect(w.get('.menu-title').classes()).toEqual(['menu-title', 'pt-3', 'text-xs', 'text-base-content/70']);
    expect(w.get('[data-sidebar-footer]').classes()).toEqual(['flex', 'flex-col', 'gap-1', 'border-t', 'border-base-300', 'p-2']);
  });

  it('titles a group only when it has a label, and shows item badges', () => {
    const w = mount(SidebarNav, { props: { groups } });
    expect(w.get('nav').attributes('aria-label')).toBeUndefined();
    expect(w.findAll('.menu-title').map(t => t.text())).toEqual(['Build']);
    expect(w.get('.badge').text()).toBe('3');
  });

  it('puts the lead groups, then the between row in a bordered strip, then the groups', () => {
    const lead = [{ items: [{ key: 'projects', label: 'Projects', to: '/o/acme/projects' }] }];
    const w = mount(SidebarNav, { props: { groups, lead }, slots: { between: '<b>Support Copilot</b>' } });
    const nav = w.get('nav').element;
    expect([...nav.children].map(c => c.tagName)).toEqual(['UL', 'DIV', 'UL']);
    expect(nav.children[1]!.className).toBe('border-y border-base-300 my-1 p-2');
    expect(nav.children[1]!.textContent).toBe('Support Copilot');
    expect(nav.children[0]!.textContent).toContain('Projects');
    expect(mount(SidebarNav, { props: { groups } }).get('nav').element.children).toHaveLength(1);
  });

  it('outlines a link on keyboard focus, open and as a rail, since the menu tint alone is not a visible focus', () => {
    for (const rail of [false, true]) {
      const link = mount(SidebarNav, { props: { groups, rail } }).get('a');
      expect(link.classes()).toEqual(expect.arrayContaining(['focus-visible:outline-2', 'focus-visible:outline-solid!', 'focus-visible:-outline-offset-2']));
    }
  });

  it('collapses to an icon rail at lg that keeps every name: tooltips, visually hidden labels, no header strip', () => {
    const w = mount(SidebarNav, { props: { groups, rail: true }, slots: { between: 'Refund Triage' } });
    expect(w.get('aside').classes()).toContain('lg:w-16');
    const link = w.get('a');
    const item = link.element.parentElement!;
    expect([...item.classList]).toEqual(['lg:tooltip', 'lg:tooltip-right']);
    expect(item.getAttribute('data-tip')).toBe(link.text());
    expect(link.classes()).toContain('lg:justify-center');
    expect(link.get('span').classes()).toContain('lg:sr-only');
    expect(w.get('nav').classes()).toContain('lg:overflow-visible');
    expect(w.get('nav > div').classes()).toContain('lg:hidden');
    const open = mount(SidebarNav, { props: { groups } });
    expect(open.get('aside').classes()).not.toContain('lg:w-16');
    expect(open.get('li').attributes('data-tip')).toBeUndefined();
    expect(open.get('nav').classes()).not.toContain('lg:overflow-visible');
  });

  it('keeps the menu padding and a plain header strip when not dense, as the agent sidebar does', () => {
    const lead = [{ items: [{ key: 'agents', label: 'Agents', to: '/agents' }] }];
    const w = mount(SidebarNav, { props: { groups, lead, dense: false }, slots: { between: 'Refund Triage' } });
    const nav = w.get('nav').element;
    expect(nav.children[1]!.className).toBe('border-y border-base-300 px-4 py-2');
    expect(w.findAll('ul').map(u => u.classes())).toEqual([['menu', 'menu-sm', 'w-full', 'text-ui'], ['menu', 'menu-sm', 'w-full', 'text-ui']]);
  });

  it('renders the top, back and footer slots only when given', () => {
    expect(mount(SidebarNav, { props: { groups } }).find('[data-sidebar-footer]').exists()).toBe(false);
    const w = mount(SidebarNav, { props: { groups }, slots: { top: 'Switcher', back: 'Back', footer: 'OAgent v1.2.3' } });
    expect(w.get('[data-sidebar-footer]').text()).toBe('OAgent v1.2.3');
    expect(w.text()).toContain('Switcher');
    expect(w.text()).toContain('Back');
  });
});
