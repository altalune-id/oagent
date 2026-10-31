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
    const trigger = mount(UserMenu, { props: { me, logoutUrl: '/logout' } }).get('[role="button"]');
    expect(trigger.attributes('aria-haspopup')).toBe('true');
    expect(trigger.attributes('aria-label')).toBeUndefined();
    expect(accessibleText(trigger.element).replace(/\s+/g, ' ').trim()).toBe('Signed in as Rina Admin');
  });

  it.each(['local', 'oidc'])('gives a %s person Profile and Sign out', (source) => {
    const w = mount(UserMenu, { props: { me: { ...me, source }, logoutUrl: '/logout' } });
    expect(w.findAll('a').map(a => a.text())).toContain('Profile');
    expect(w.find('form[action="/logout"]').exists()).toBe(true);
  });

  it('gives an API key none of them', () => {
    const w = mount(UserMenu, { props: { me: { ...me, source: 'apikey', userId: '' }, logoutUrl: '/logout' } });
    expect(w.find('form[action="/logout"]').exists()).toBe(false);
  });

  it('shows nothing without a signed-in person', () => {
    expect(mount(UserMenu, { props: { me: null, logoutUrl: '/logout' } }).find('[role="button"]').exists()).toBe(false);
  });
});
