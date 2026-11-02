// @vitest-environment happy-dom
import { mount } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import UserMenu from './UserMenu.vue';

const me = { source: 'local', userId: 'F5pLm2XwQ8kRt3VnZ7bYc', name: 'Rina Admin', email: 'rina@acme.test' };

function accessibleText(el: Element): string {
  if (el.getAttribute('aria-hidden') === 'true') {
    return '';
  }
  return [...el.childNodes].map(n => (n.nodeType === 3 ? n.textContent ?? '' : accessibleText(n as Element))).join(' ');
}

describe('userMenu', () => {
  it('names its trigger "Signed in as" plus the visible name, without the avatar initials', () => {
    const trigger = mount(UserMenu, { props: { me, logoutUrl: '/logout' } }).get('button[popovertarget]');
    expect(trigger.attributes('aria-haspopup')).toBe('true');
    expect(trigger.attributes('aria-label')).toBeUndefined();
    expect(accessibleText(trigger.element).replace(/\s+/g, ' ').trim()).toBe('Signed in as Rina Admin');
  });

  it.each(['local', 'oidc'])('gives a %s person Profile and Sign out', (source) => {
    const w = mount(UserMenu, { props: { me: { ...me, source }, logoutUrl: '/logout' } });
    expect(w.findAll('a').map(a => a.text())).toContain('Profile');
    expect(w.find('form[action="/logout"]').exists()).toBe(true);
  });

  it('opens upward from the sidebar row as a dense menu titled by the email, with the version and a sign-out button', () => {
    const w = mount(UserMenu, { props: { me, logoutUrl: '/logout', placement: 'top', version: '1.2.3' } });
    expect(w.get('[popover]').classes()).toContain('dropdown-top');
    expect(w.get('button[popovertarget]').classes()).toEqual(expect.arrayContaining(['w-full', 'justify-start', 'text-ui']));
    expect(w.get('ul').classes()).toEqual(['menu', 'menu-sm', 'w-full', 'p-0', 'text-ui']);
    expect(w.get('.menu-title').text()).toContain('rina@acme.test');
    expect(w.get('.menu-title').text()).not.toContain('OAgent v1.2.3');
    expect(w.get('ul + p').classes()).toEqual(['px-2', 'pt-1', 'text-xs', 'text-base-content/70']);
    expect(w.get('ul + p').text()).toBe('OAgent v1.2.3');
    const out = w.get('button[type="submit"]');
    expect(w.get(`form#${out.attributes('form')}`).attributes('action')).toBe('/logout');
  });

  it('gives an API key none of them', () => {
    const w = mount(UserMenu, { props: { me: { ...me, source: 'apikey', userId: '' }, logoutUrl: '/logout' } });
    expect(w.find('form[action="/logout"]').exists()).toBe(false);
  });

  it('shows nothing without a signed-in person', () => {
    expect(mount(UserMenu, { props: { me: null, logoutUrl: '/logout' } }).find('button[popovertarget]').exists()).toBe(false);
  });

  it('links My API keys only when given its org-scoped URL', () => {
    expect(mount(UserMenu, { props: { me, logoutUrl: '/logout' } }).findAll('a').map(a => a.text())).not.toContain('My API keys');
    const w = mount(UserMenu, { props: { me, logoutUrl: '/logout', apiKeysUrl: '/o/acme/settings/personal-keys' } });
    expect(w.findAll('a').map(a => a.text())).toContain('My API keys');
  });
});
