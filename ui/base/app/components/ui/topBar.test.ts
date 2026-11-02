// @vitest-environment happy-dom
import { mount, RouterLinkStub } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import CommandPalette from './CommandPalette.vue';
import TopBar from './TopBar.vue';

const crumbs = [{ label: 'Acme', to: '/o/acme' }, { label: 'acme-prod', to: '/o/acme/projects/acme-prod', mono: true }, { label: 'Overview' }];

describe('topBar', () => {
  it('is the design navbar: menu button on phones, the trail, both search buttons and the quiet controls', async () => {
    const w = mount(TopBar, { props: { crumbs }, slots: { controls: '<select aria-label="Theme family"></select>' } });
    expect(w.get('.navbar').classes()).toEqual(['navbar', 'min-h-12', 'gap-2', 'border-b', 'border-base-300', 'bg-base-100', 'px-3', 'md:px-4']);
    const buttons = w.findAll('button');
    expect(buttons.map(b => b.attributes('aria-label'))).toEqual(['Toggle navigation', 'Search and jump', 'Search and jump, Command K']);
    expect(buttons[0]!.classes()).toContain('lg:hidden');
    expect(buttons[0]!.attributes('aria-controls')).toBe('app-sidebar');
    expect(buttons[0]!.attributes('aria-expanded')).toBe('false');
    expect(buttons[2]!.get('kbd').classes()).toEqual(['kbd', 'kbd-xs']);
    expect(w.get('nav').classes()).toEqual(expect.arrayContaining(['flex-1', 'max-md:hidden']));
    expect(w.get('[aria-current="page"]').text()).toBe('Overview');
    expect(w.find('.hidden.md\\:block select').exists()).toBe(true);
    await buttons[0]!.trigger('click');
    await buttons[1]!.trigger('click');
    await buttons[2]!.trigger('click');
    expect(w.emitted('menu')).toHaveLength(1);
    expect(w.emitted('search')).toHaveLength(2);
  });
});

describe('topBar on a phone', () => {
  it('names the place in mono with its environment badge instead of the trail', () => {
    const w = mount(TopBar, { props: { crumbs, place: { label: 'acme-prod', mono: true, badge: 'Production' } } });
    const name = w.get('.md\\:hidden .truncate');
    expect(name.text()).toBe('acme-prod');
    expect(name.classes()).toEqual(['truncate', 'font-medium', 'font-mono', 'text-xs']);
    expect(w.get('.badge.badge-sm.md\\:hidden').text()).toBe('Production');
    expect(mount(TopBar, { props: { crumbs, place: { label: 'Acme' } } }).find('.badge').exists()).toBe(false);
  });
});

describe('commandPalette', () => {
  const items = [{ key: 'a', group: 'Acme', label: 'Members', to: '/o/acme/settings/members' }, { key: 'b', group: 'acme-prod', mono: true, label: 'Clients', to: '/o/acme/projects/acme-prod/clients' }];

  it('opens when mounted open, and draws the project handle in mono before the page', async () => {
    const w = mount(CommandPalette, { props: { items, label: 'Search and jump', open: true }, global: { stubs: { NuxtLink: RouterLinkStub } }, attachTo: document.body });
    await w.vm.$nextTick();
    expect((w.get('dialog').element as HTMLDialogElement).open).toBe(true);
    const handle = w.findAllComponents(RouterLinkStub)[1]!;
    expect(handle.text()).toBe('acme-prod / Clients');
    expect(handle.get('.font-mono').classes()).toEqual(['font-mono', 'text-xs']);
    w.unmount();
  });

  it('filters the pages by the search and closes when one is picked', async () => {
    const w = mount(CommandPalette, { props: { items, 'label': 'Search and jump', 'empty': 'No page matches.', 'open': false, 'onUpdate:open': (v: boolean) => w.setProps({ open: v }) }, global: { stubs: { NuxtLink: RouterLinkStub } }, attachTo: document.body });
    expect(w.get('dialog').classes()).toEqual(['modal', 'modal-top', 'sm:modal-middle']);
    expect(w.get('.modal-box').classes()).toEqual(['modal-box', 'p-0', 'sm:max-w-lg']);
    await w.get('input[type="search"]').setValue('prod clients');
    expect(w.findAllComponents(RouterLinkStub).map(l => l.props('to'))).toEqual(['/o/acme/projects/acme-prod/clients']);
    await w.get('input[type="search"]').setValue('nothing');
    expect(w.get('.menu-disabled').text()).toBe('No page matches.');
    await w.setProps({ open: true });
    await w.get('input[type="search"]').setValue('');
    await w.findComponent(RouterLinkStub).trigger('click');
    expect(w.props('open')).toBe(false);
    w.unmount();
  });

  it('keeps the navigation toggle at lg with panel-left when the page has a rail', () => {
    const w = mount(TopBar, { props: { crumbs: [], rail: true, menuOpen: true } });
    const toggle = w.get('button[aria-controls]');
    expect(toggle.classes()).not.toContain('lg:hidden');
    expect(toggle.attributes('aria-expanded')).toBe('true');
    expect(toggle.find('[data-icon="lucide:panel-left"]').exists()).toBe(true);
    expect(mount(TopBar, { props: { crumbs: [] } }).get('button[aria-controls]').classes()).toContain('lg:hidden');
  });
});
