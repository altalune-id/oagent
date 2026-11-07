// @vitest-environment happy-dom
import { mount } from '@vue/test-utils';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { nextTick } from 'vue';
import AppShell from './AppShell.vue';

function render(open = false) {
  return mount(AppShell, {
    props: { 'open': open, 'onUpdate:open': () => {} },
    slots: { default: '<main><button id="page-button">Page</button></main>', sidebar: '<nav><a href="/one">One</a></nav>' },
    attachTo: document.body,
  });
}

let phone = true;

beforeEach(() => {
  phone = true;
  vi.spyOn(window, 'matchMedia').mockImplementation(() => ({ matches: !phone, addEventListener: () => {}, removeEventListener: () => {} }) as unknown as MediaQueryList);
});

afterEach(() => {
  document.body.innerHTML = '';
});

describe('appShell', () => {
  it('is a daisyUI drawer that stays open from md up, with an overlay tied to the toggle and hidden from screen readers, which close it with Escape', () => {
    const w = render();
    expect(w.classes()).toEqual(expect.arrayContaining(['drawer', 'md:drawer-open']));
    const toggle = w.get('input.drawer-toggle');
    expect(w.get('label.drawer-overlay').attributes('for')).toBe(toggle.attributes('id'));
    expect(w.get('label.drawer-overlay').attributes('aria-hidden')).toBe('true');
    expect(w.get('label.drawer-overlay').attributes('aria-label')).toBeUndefined();
    expect(w.get('.drawer-side').attributes('id')).toBe('app-sidebar');
    w.unmount();
  });

  it('runs full height from lg up when it owns the top bar, and stays under the navbar otherwise', () => {
    const under = render();
    expect(under.get('.drawer-side').classes()).toEqual(expect.arrayContaining(['top-14', 'h-[calc(100dvh-3.5rem)]']));
    under.unmount();
    const full = mount(AppShell, { props: { full: true, breakpoint: 'lg' }, slots: { default: '<main>Page</main>', sidebar: '<nav>Nav</nav>' } });
    expect(full.classes()).toEqual(expect.arrayContaining(['drawer', 'lg:drawer-open']));
    expect(full.classes()).not.toContain('md:drawer-open');
    expect(full.get('.drawer-side').classes()).toEqual(['drawer-side', 'z-40']);
    expect(window.matchMedia).toHaveBeenCalledWith('(min-width: 64rem)');
    full.unmount();
  });

  it('moves focus into the sidebar on open, makes the page inert, and closes on Escape with focus returned', async () => {
    const w = render();
    const page = w.get('#page-button').element as HTMLButtonElement;
    page.focus();
    await w.setProps({ open: true });
    await nextTick();
    expect(document.activeElement?.textContent).toBe('One');
    expect(w.get('.drawer-content').attributes('inert')).toBeDefined();
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
    expect(w.emitted('update:open')?.at(-1)).toEqual([false]);
    await w.setProps({ open: false });
    expect(document.activeElement).toBe(page);
    expect(w.get('.drawer-content').attributes('inert')).toBeUndefined();
    w.unmount();
  });

  it('focuses the first sidebar link only once the drawer side is visible, as it fades in on a phone', async () => {
    const w = render();
    const side = w.get('.drawer-side').element as HTMLElement;
    side.style.visibility = 'hidden';
    await w.setProps({ open: true });
    await nextTick();
    expect(document.activeElement?.textContent).not.toBe('One');
    side.style.visibility = 'visible';
    await new Promise(resolve => setTimeout(resolve, 60));
    expect(document.activeElement?.textContent).toBe('One');
    w.unmount();
  });

  it('ignores other keys and leaves focus alone when it already moved out of the sidebar', async () => {
    phone = false;
    const w = render();
    await w.setProps({ open: true });
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter' }));
    expect(w.emitted('update:open')).toBeUndefined();
    (w.get('#page-button').element as HTMLButtonElement).focus();
    await w.setProps({ open: false });
    expect(document.activeElement?.id).toBe('page-button');
    w.unmount();
  });

  it('keeps the page usable from md up, where the sidebar is always shown', async () => {
    phone = false;
    const w = render();
    await w.setProps({ open: true });
    expect(w.get('.drawer-content').attributes('inert')).toBeUndefined();
    w.unmount();
  });
});
