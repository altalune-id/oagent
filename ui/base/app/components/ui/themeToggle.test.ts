// @vitest-environment happy-dom
import { mount } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import ThemeSelect from './ThemeSelect.vue';
import ThemeToggle from './ThemeToggle.vue';

describe('themeToggle', () => {
  it('is a quiet named icon button that reports dark as pressed', async () => {
    const w = mount(ThemeToggle, { props: { mode: 'light', label: 'Switch between light and dark' } });
    const button = w.get('button');
    expect(button.classes()).toEqual(['btn', 'btn-xs', 'btn-ghost', 'btn-square']);
    expect(button.attributes('aria-label')).toBe('Switch between light and dark');
    expect(button.attributes('aria-pressed')).toBe('false');
    expect(w.find('[data-icon="lucide:sun-moon"]').exists()).toBe(true);
    await w.setProps({ mode: 'dark' });
    expect(button.attributes('aria-pressed')).toBe('true');
    await button.trigger('click');
    expect(w.emitted('toggle')).toHaveLength(1);
  });
});

describe('themeSelect', () => {
  it('is a labelled quiet select of the theme families that emits the picked one', async () => {
    const families = [{ id: 'othra', label: 'Othra', theme: 'othra-light' }, { id: 'night', label: 'Night', theme: 'night-dark' }];
    const w = mount(ThemeSelect, { props: { families, modelValue: 'othra', label: 'Theme family' } });
    const select = w.get('select');
    expect(select.classes()).toEqual(['select', 'select-xs', 'select-ghost', 'w-28', 'text-xs']);
    expect(w.get('label .sr-only').text()).toBe('Theme family');
    expect(w.find('label select').exists()).toBe(true);
    await select.setValue('night');
    expect(w.emitted('select')).toEqual([['night']]);
  });
});
