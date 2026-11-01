// @vitest-environment happy-dom
import { mount } from '@vue/test-utils';
import { describe, expect, it, vi } from 'vitest';
import AppDropdown from './AppDropdown.vue';

function render(props: Record<string, unknown> = {}) {
  return mount(AppDropdown, {
    props: { label: 'Your account', ...props },
    slots: { trigger: '<span>ana@acme.test</span>', default: '<ul class="menu"><li><button type="button">Sign out</button></li><li><button type="button" data-keep-open>Pin</button></li></ul>' },
  });
}

describe('appDropdown', () => {
  it('opens a popover from a real button tied to it by id and anchor', () => {
    const w = render();
    const trigger = w.get('button[popovertarget]');
    const panel = w.get('[popover]');
    const id = panel.attributes('id')!;
    expect(trigger.attributes('popovertarget')).toBe(id);
    expect(trigger.attributes('aria-haspopup')).toBe('true');
    expect(trigger.attributes('aria-expanded')).toBe('false');
    expect(trigger.attributes('style')).toContain(`--${id}`);
    expect(panel.attributes('style')).toContain(`--${id}`);
    expect(panel.classes()).toEqual(expect.arrayContaining(['dropdown', 'dropdown-start', 'w-56']));
  });

  it('keeps the visible text in the accessible name (WCAG 2.5.3) and adds the hidden label', () => {
    const trigger = render().get('button[popovertarget]');
    expect(trigger.attributes('aria-label')).toBeUndefined();
    expect(trigger.get('.sr-only').text()).toBe('Your account');
    expect(trigger.text()).toContain('ana@acme.test');
    expect(render({ label: '' }).find('.sr-only').exists()).toBe(false);
  });

  it('places the panel at the end and disables the trigger', () => {
    const w = render({ disabled: true, placement: 'end', contentClass: 'w-48' });
    expect(w.get('button[popovertarget]').attributes('disabled')).toBeDefined();
    expect(w.get('[popover]').classes()).toEqual(expect.arrayContaining(['dropdown-end', 'w-48']));
  });

  it('tracks aria-expanded from the toggle event and closes when an item is picked', async () => {
    const w = render();
    const panel = w.get('[popover]');
    const hide = vi.fn();
    (panel.element as HTMLElement).hidePopover = hide;
    panel.element.dispatchEvent(Object.assign(new Event('toggle'), { newState: 'open' }));
    await w.vm.$nextTick();
    expect(w.get('button[popovertarget]').attributes('aria-expanded')).toBe('true');
    await panel.findAll('button')[1]!.trigger('click');
    expect(hide).not.toHaveBeenCalled();
    await panel.findAll('button')[0]!.trigger('click');
    expect(hide).toHaveBeenCalledOnce();
    await panel.trigger('click');
    expect(hide).toHaveBeenCalledOnce();
  });

  it('opens on mount when forced open', () => {
    const proto = HTMLElement.prototype as { showPopover?: () => void };
    const before = proto.showPopover;
    const show = vi.fn();
    proto.showPopover = show;
    try {
      render({ open: true });
    } finally {
      proto.showPopover = before;
    }
    expect(show).toHaveBeenCalledOnce();
  });
});
