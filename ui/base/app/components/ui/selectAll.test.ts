// @vitest-environment happy-dom
import { mount } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import { selectAllState, withAll } from './selectAll';
import SelectAllCheckbox from './SelectAllCheckbox.vue';

describe('select all', () => {
  it('reports none, some or all of a group as on', () => {
    expect([selectAllState(0, 3), selectAllState(1, 3), selectAllState(3, 3), selectAllState(0, 0)]).toEqual(['none', 'some', 'all', 'none']);
  });

  it('adds every option once and removes only the group, keeping values outside it', () => {
    expect(withAll(['x', 'a'], ['a', 'b'], true)).toEqual(['x', 'a', 'b']);
    expect(withAll(['x', 'a', 'b'], ['a', 'b'], false)).toEqual(['x']);
  });

  it('is a labelled checkbox that shows each state, mixed for some', () => {
    const box = (state: 'all' | 'some' | 'none') => {
      const w = mount(SelectAllCheckbox, { props: { state, label: 'Select all scopes' } });
      const input = w.get('input');
      const el = input.element as HTMLInputElement;
      return { label: w.text(), checked: el.checked, mixed: el.indeterminate, aria: input.attributes('aria-checked') };
    };
    expect(box('none')).toEqual({ label: 'Select all scopes', checked: false, mixed: false, aria: undefined });
    expect(box('some')).toEqual({ label: 'Select all scopes', checked: false, mixed: true, aria: 'mixed' });
    expect(box('all')).toEqual({ label: 'Select all scopes', checked: true, mixed: false, aria: undefined });
  });

  it('puts a screen-reader context before the label, separated by a space', () => {
    const w = mount(SelectAllCheckbox, { props: { state: 'none', label: 'Turn on all 3', context: 'walk-stub:' } });
    expect(w.text()).toBe('walk-stub: Turn on all 3');
    expect(w.get('.sr-only').text()).toBe('walk-stub:');
  });

  it('emits the new value, takes a size and can be disabled', async () => {
    const w = mount(SelectAllCheckbox, { props: { state: 'some', label: 'All', size: 'xs', disabled: true } });
    expect(w.get('input').classes()).toContain('checkbox-xs');
    expect(w.get('input').attributes('disabled')).toBeDefined();
    await w.setProps({ disabled: false });
    await w.get('input').setValue(true);
    await w.get('input').setValue(false);
    expect(w.emitted('change')).toEqual([[true], [false]]);
  });
});
