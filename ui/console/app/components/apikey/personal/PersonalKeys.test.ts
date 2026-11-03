// @vitest-environment happy-dom
import { mount } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import PersonalKeys from './PersonalKeys.vue';

const required = { caption: 'Personal API keys', actionsFor: (name: string) => `Actions for ${name}`, actionsColumn: 'Actions' };

const keys = [
  { rowKey: 'k1', name: 'Laptop', secretHint: 'a1b2', scopes: ['Read agents'], lastUsed: '', expires: 'Jan 14, 2027', expiry: { kind: 'soon' as const, days: 5 }, revoked: false },
  { rowKey: 'k2', name: 'Retired', secretHint: 'g7h8', scopes: ['Read webhooks'], lastUsed: '30 days ago', expires: '', expiry: { kind: 'ok' as const }, revoked: true },
];

describe('personal keys', () => {
  it('lists keys in the shared table with status dots, and offers Revoke only on active keys, behind a confirm', async () => {
    const w = mount(PersonalKeys, { props: { keys, ...required }, attachTo: document.body });
    expect(w.get('caption').text()).toBe('Personal API keys');
    const rows = w.findAll('tbody tr');
    expect(rows[0]!.text()).toContain('Never used');
    expect(rows[0]!.text()).toContain('Expires in 5 days');
    expect(rows[0]!.find('.badge').exists()).toBe(false);
    expect(rows[1]!.text()).toContain('Revoked');
    expect(rows[0]!.get('button[popovertarget] .sr-only').text()).toBe('Actions for Laptop');
    expect(rows[1]!.find('button[popovertarget]').exists()).toBe(false);
    await rows[0]!.get('[popover] button').trigger('click');
    expect(w.emitted('revoke')).toBeUndefined();
    const dialog = [...document.querySelectorAll('dialog')].find(d => d.open)!;
    expect(dialog.textContent).toContain('Laptop');
    [...dialog.querySelectorAll('button')].find(b => b.textContent!.trim() === 'Revoke')!.click();
    expect(w.emitted('revoke')).toEqual([['a1b2']]);
    w.unmount();
  });

  it('shows the empty state when there are no keys', () => {
    const w = mount(PersonalKeys, { props: { keys: [], ...required } });
    expect(w.get('h2').text()).toBe('You have no personal API keys in this organization yet.');
  });
});
