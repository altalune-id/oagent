// @vitest-environment happy-dom
import type { Component } from 'vue';
import { mount } from '@vue/test-utils';
import { describe, expect, it, vi } from 'vitest';
import { nextTick } from 'vue';
import { countTone } from './markdownEditor';
import MarkdownEditor from './MarkdownEditor.vue';

describe('countTone', () => {
  it('warns from 90% of max and errors over it', () => {
    expect(countTone(89, 100)).toBe('ok');
    expect(countTone(90, 100)).toBe('warning');
    expect(countTone(100, 100)).toBe('warning');
    expect(countTone(101, 100)).toBe('error');
  });
});

const Editor: Component = MarkdownEditor;

function render(props: Record<string, unknown> = {}) {
  const w = mount(Editor, {
    props: { 'label': 'Instructions', 'max': 10, 'modelValue': '', 'onUpdate:modelValue': (v: string) => w.setProps({ modelValue: v }), ...props },
    attachTo: document.body,
  });
  return w;
}

describe('markdownEditor', () => {
  it('names the tablist and switches tabs by click and arrow keys with a roving tabindex', async () => {
    const w = render({ modelValue: '# Hi' });
    const list = w.get('[role="tablist"]');
    expect(list.attributes('aria-label')).toBe('Instructions view');
    expect(list.classes()).toEqual(['tabs', 'tabs-border', 'tabs-xs']);
    const [write, preview] = w.findAll('[role="tab"]');
    const panel = w.get('[role="tabpanel"]');
    expect(write!.attributes()).toMatchObject({ 'aria-selected': 'true', 'tabindex': '0', 'aria-controls': panel.attributes('id') });
    expect(preview!.attributes()).toMatchObject({ 'aria-selected': 'false', 'tabindex': '-1' });
    expect(panel.attributes('aria-labelledby')).toBe(write!.attributes('id'));
    expect(w.find('textarea').exists()).toBe(true);

    await write!.trigger('keydown', { key: 'ArrowRight' });
    await nextTick();
    expect(preview!.attributes('aria-selected')).toBe('true');
    expect(document.activeElement).toBe(preview!.element);
    expect(w.get('[role="tabpanel"]').attributes('aria-labelledby')).toBe(preview!.attributes('id'));
    expect(w.find('textarea').exists()).toBe(false);
    expect(w.get('[role="tabpanel"] h1').text()).toBe('Hi');

    await preview!.trigger('keydown', { key: 'ArrowLeft' });
    await nextTick();
    expect(document.activeElement).toBe(write!.element);
    await write!.trigger('keydown', { key: 'Enter' });
    expect(write!.attributes('aria-selected')).toBe('true');
    await preview!.trigger('click');
    expect(preview!.attributes('aria-selected')).toBe('true');
  });

  it('counts code points, so an emoji is one, and describes the field by hint, error and count', () => {
    const w = render({ modelValue: 'ab😀', hint: 'Rules', error: 'Too long' });
    const count = w.get('.tabular-nums');
    expect(count.text()).toBe('3 / 10');
    const ids = w.get('textarea').attributes('aria-describedby')!.split(' ');
    expect(ids.map(id => document.getElementById(id)!.textContent!.trim())).toEqual(['Rules', 'Too long', '3 / 10']);
    expect(w.get('textarea').attributes('maxlength')).toBeUndefined();
    expect([...(w.get('textarea').element as HTMLTextAreaElement).labels!].map(l => l.textContent!.trim())).toEqual(['Instructions']);
  });

  it('colors the count muted, warning from 90% and error over max, marking the field invalid over max', async () => {
    const w = render({ modelValue: '12345678' });
    expect(w.get('.tabular-nums').classes()).toContain('text-base-content/70');
    expect(w.get('textarea').attributes('aria-invalid')).toBe('false');
    await w.setProps({ modelValue: '123456789' });
    expect(w.get('.tabular-nums').classes()).toContain('text-warning');
    await w.setProps({ modelValue: '12345678901' });
    expect(w.get('.tabular-nums').classes()).toContain('text-error');
    expect(w.get('textarea').classes()).toContain('textarea-error');
    expect(w.get('textarea').attributes('aria-invalid')).toBe('true');
  });

  it('edits through v-model', async () => {
    const w = render();
    await w.get('textarea').setValue('Be kind.');
    expect(w.emitted('update:modelValue')![0]).toEqual(['Be kind.']);
  });

  it('shows a muted note when there is nothing to preview', async () => {
    const w = render({ modelValue: '  ', labels: { write: 'Tulis', preview: 'Pratinjau', done: 'Selesai', empty: 'Tidak ada pratinjau' } });
    expect(w.findAll('[role="tab"]').map(t => t.text())).toEqual(['Tulis', 'Pratinjau']);
    await w.findAll('[role="tab"]')[1]!.trigger('click');
    expect(w.get('[role="tabpanel"]').text()).toBe('Tidak ada pratinjau');
    expect(w.get('[role="tabpanel"] > div').classes()).toContain('text-base-content/70');
  });

  it('expands into a wide modal on the same value, focuses its field and returns focus on Done', async () => {
    const w = render({ modelValue: 'Hello', expandLabel: 'Perluas Instruksi' });
    const button = w.get('button[aria-label="Perluas Instruksi"]');
    const dialog = w.get('dialog').element as HTMLDialogElement;
    expect(dialog.open).toBe(false);
    await button.trigger('click');
    await nextTick();
    await nextTick();
    expect(dialog.open).toBe(true);
    expect(w.get('.modal-box').classes()).toContain('max-w-3xl');
    const field = w.get('dialog textarea');
    expect(document.activeElement).toBe(field.element);
    expect((field.element as HTMLTextAreaElement).value).toBe('Hello');
    expect(field.classes()).toContain('max-h-[70vh]');
    await field.setValue('Hello there');
    expect(w.emitted('update:modelValue')!.at(-1)).toEqual(['Hello there']);
    expect(w.findAll('dialog button[aria-label^="Expand"]')).toHaveLength(0);
    const openAtFocus: boolean[] = [];
    const focus = HTMLElement.prototype.focus;
    vi.spyOn(button.element as HTMLElement, 'focus').mockImplementation(function (this: HTMLElement) {
      openAtFocus.push(dialog.open);
      focus.call(this);
    });
    await w.findAll('dialog .modal-action button').at(0)!.trigger('click');
    await nextTick();
    expect(dialog.open).toBe(false);
    expect(openAtFocus).toEqual([false]);
    expect(document.activeElement).toBe(button.element);
  });

  it('shows only the rendered preview when read-only', () => {
    const w = render({ modelValue: '**bold**', readonly: true });
    expect(w.find('[role="tablist"]').exists()).toBe(false);
    expect(w.find('textarea').exists()).toBe(false);
    expect(w.find('dialog').exists()).toBe(false);
    expect(w.get('strong').text()).toBe('bold');
  });
});
