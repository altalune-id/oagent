// @vitest-environment happy-dom
import { mount } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import AppDropdown from './AppDropdown.vue';

function render(props: Record<string, unknown> = {}) {
  return mount(AppDropdown, {
    props: { label: 'Your account', ...props },
    slots: { trigger: '<span>ana@acme.test</span>', default: '<ul tabindex="-1" class="dropdown-content"><li>Sign out</li></ul>' },
  });
}

describe('appDropdown', () => {
  it('opens from a focusable div that Safari can focus, announced as a menu button', () => {
    const trigger = render().get('[role="button"]');
    expect(trigger.element.tagName).toBe('DIV');
    expect(trigger.attributes('tabindex')).toBe('0');
    expect(trigger.attributes('aria-haspopup')).toBe('true');
  });

  it('keeps the visible text in the accessible name (WCAG 2.5.3) and adds the hidden label', () => {
    const trigger = render().get('[role="button"]');
    expect(trigger.attributes('aria-label')).toBeUndefined();
    expect(trigger.get('.sr-only').text()).toBe('Your account');
    expect(trigger.text()).toContain('ana@acme.test');
    expect(render({ label: '' }).find('.sr-only').exists()).toBe(false);
  });

  it('takes itself out of the tab order when disabled, and can be forced open', () => {
    const w = render({ disabled: true, open: true });
    const trigger = w.get('[role="button"]');
    expect(trigger.attributes('tabindex')).toBe('-1');
    expect(trigger.attributes('aria-disabled')).toBe('true');
    expect(trigger.classes()).toContain('btn-disabled');
    expect(w.classes()).toContain('dropdown-open');
  });
});
