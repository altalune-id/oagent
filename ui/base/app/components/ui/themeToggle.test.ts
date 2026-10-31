// @vitest-environment happy-dom
import { mount } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import ThemeToggle from './ThemeToggle.vue';

describe('themeToggle', () => {
  it('names the mode it switches to, even when the text is hidden on a phone', async () => {
    const w = mount(ThemeToggle, { props: { mode: 'light', lightLabel: 'Light', darkLabel: 'Dark' } });
    expect(w.get('button').attributes('aria-label')).toBe('Dark');
    await w.setProps({ mode: 'dark' });
    expect(w.get('button').attributes('aria-label')).toBe('Light');
    await w.get('button').trigger('click');
    expect(w.emitted('toggle')).toHaveLength(1);
  });
});
