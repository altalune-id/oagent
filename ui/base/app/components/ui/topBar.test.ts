// @vitest-environment happy-dom
import { mount, RouterLinkStub } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import CommandPalette from './CommandPalette.vue';
import TopBar from './TopBar.vue';

const crumbs = [{ label: 'Acme', to: '/o/acme' }, { label: 'acme-prod', to: '/o/acme/projects/acme-prod', mono: true }, { label: 'Overview' }];

describe('topBar', () => {
  it('is the design navbar: menu button on phones, the trail, both search buttons and the quiet controls', async () => {
    const w = mount(TopBar, { props: { crumbs }, slots: { controls: '<select aria-label="Theme family"></select>' } });
    expect(w.get('.navbar').classes()).toEqual(['navbar', 'min-h-12', 'gap-2', 'border-b', 'border-base-300', 'bg-base-100', 'px-4']);
    const buttons = w.findAll('button');
    expect(buttons.map(b => b.attributes('aria-label'))).toEqual(['Toggle navigation', 'Search and jump', 'Search and jump, Command K']);
    expect(buttons[0]!.classes()).toContain('lg:hidden');
    expect(buttons[2]!.get('kbd').classes()).toEqual(['kbd', 'kbd-xs']);
    expect(w.get('nav').classes()).toContain('flex-1');
    expect(w.get('[aria-current="page"]').text()).toBe('Overview');
    expect(w.find('.hidden.md\\:block select').exists()).toBe(true);
    await buttons[0]!.trigger('click');
    await buttons[1]!.trigger('click');
    await buttons[2]!.trigger('click');
    expect(w.emitted('menu')).toHaveLength(1);
    expect(w.emitted('search')).toHaveLength(2);
  });
});

describe('commandPalette', () => {
  const items = [{ key: 'a', label: 'Acme / Members', to: '/o/acme/settings/members' }, { key: 'b', label: 'acme-prod / Clients', to: '/o/acme/projects/acme-prod/clients' }];

  it('filters the pages by the search and closes when one is picked', async () => {
    const w = mount(CommandPalette, { props: { items, 'label': 'Search and jump', 'empty': 'No page matches.', 'open': false, 'onUpdate:open': (v: boolean) => w.setProps({ open: v }) }, global: { stubs: { NuxtLink: RouterLinkStub } }, attachTo: document.body });
    expect(w.get('dialog').classes()).toEqual(['modal', 'modal-top', 'sm:modal-middle']);
    expect(w.get('.modal-box').classes()).toEqual(['modal-box', 'p-0', 'sm:max-w-lg']);
    await w.get('input[type="search"]').setValue('client');
    expect(w.findAllComponents(RouterLinkStub).map(l => l.props('to'))).toEqual(['/o/acme/projects/acme-prod/clients']);
    await w.get('input[type="search"]').setValue('nothing');
    expect(w.get('.menu-disabled').text()).toBe('No page matches.');
    await w.setProps({ open: true });
    await w.get('input[type="search"]').setValue('');
    await w.findComponent(RouterLinkStub).trigger('click');
    expect(w.props('open')).toBe(false);
    w.unmount();
  });
});
