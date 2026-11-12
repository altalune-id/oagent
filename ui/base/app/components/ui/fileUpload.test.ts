// @vitest-environment happy-dom
import type { UploadItem } from './upload';
import { mount } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import FileUpload from './FileUpload.vue';

const maxBytes = 10 * 1024 * 1024;
const labels = {
  label: 'Choose files',
  hint: 'PDF, DOCX, Markdown or TXT, up to 10 MB each.',
  drop: 'or drop files here',
  queue: 'Uploads',
  states: { queued: 'Waiting', uploading: 'Uploading', done: 'Uploaded', failed: 'Failed' },
};

const items: UploadItem[] = [
  { key: 'a', name: 'refund-policy.pdf', size: '1.2 MB', state: 'uploading', progress: 0.42 },
  { key: 'b', name: 'faq.md', size: '3 kB', state: 'done', progress: 1 },
  { key: 'c', name: 'photo.png', size: '2 MB', state: 'failed', progress: 0, error: 'This file type isn\'t supported.' },
];

function files(...names: string[]) {
  return names.map(n => new File(['x'], n));
}

describe('fileUpload', () => {
  it('labels the native file input with the accept list and the limits hint', () => {
    const w = mount(FileUpload, { props: { ...labels, accept: '.pdf,.docx,.md,.txt', maxBytes, items: [] } });
    const input = w.get('input[type="file"]');
    expect(input.attributes('accept')).toBe('.pdf,.docx,.md,.txt');
    expect(input.attributes('multiple')).toBeDefined();
    expect(w.get('label').element.contains(input.element)).toBe(true);
    expect(w.get('label').text()).toBe('Choose files');
    expect(w.get('label').classes()).toContain('btn');
    expect(input.classes()).toContain('sr-only');
    expect(w.get(`#${input.attributes('aria-describedby')}`).text()).toBe(labels.hint);
    expect(w.find('ul.list').exists()).toBe(false);
  });

  it('emits the chosen files, then clears the input so the same file can be picked again', async () => {
    const w = mount(FileUpload, { props: { ...labels, accept: '', maxBytes, items: [] } });
    const input = w.get<HTMLInputElement>('input[type="file"]');
    Object.defineProperty(input.element, 'files', { value: files('a.pdf', 'b.md'), configurable: true });
    await input.trigger('change');
    expect(w.emitted('select')![0]![0]).toEqual([expect.objectContaining({ name: 'a.pdf' }), expect.objectContaining({ name: 'b.md' })]);
    expect(input.element.value).toBe('');
  });

  it('takes dropped files and marks the zone while a drag is over it', async () => {
    const w = mount(FileUpload, { props: { ...labels, accept: '', maxBytes, items: [] } });
    const zone = w.get('[data-dropzone]');
    await zone.trigger('dragover', { dataTransfer: { files: [] } });
    expect(zone.classes()).toContain('sm:border-primary');
    await zone.trigger('dragleave');
    expect(zone.classes()).not.toContain('sm:border-primary');
    await zone.trigger('drop', { dataTransfer: { files: files('c.txt') } });
    expect(w.emitted('select')![0]![0]).toEqual([expect.objectContaining({ name: 'c.txt' })]);
  });

  it('ignores drops and disables the input when disabled', async () => {
    const w = mount(FileUpload, { props: { ...labels, accept: '', maxBytes, items: [], disabled: true } });
    expect(w.get('input[type="file"]').attributes('disabled')).toBeDefined();
    expect(w.get('label').classes()).toContain('btn-disabled');
    await w.get('[data-dropzone]').trigger('drop', { dataTransfer: { files: files('c.txt') } });
    expect(w.emitted('select')).toBeUndefined();
  });

  it('lists each file with its size, a progress bar and percent while uploading, and the state in words', () => {
    const w = mount(FileUpload, { props: { ...labels, accept: '', maxBytes, items } });
    expect(w.get('ul').attributes('aria-label')).toBe('Uploads');
    const rows = w.get('ul').findAll('li');
    expect(rows.map(r => r.get('[data-name]').text())).toEqual(['refund-policy.pdf', 'faq.md', 'photo.png']);
    expect(rows[0]!.text()).toContain('1.2 MB');
    const bar = rows[0]!.get('progress');
    expect([bar.attributes('value'), bar.attributes('max'), bar.attributes('aria-label')]).toEqual(['42', '100', 'refund-policy.pdf']);
    expect(rows[0]!.get('[data-state]').text()).toBe('Uploading');
    expect(rows[0]!.get('[data-percent]').text()).toBe('42%');
    expect(rows[1]!.find('progress').exists()).toBe(false);
    expect(rows[1]!.get('[data-state]').text()).toBe('Uploaded');
    expect(rows[2]!.get('[data-state]').text()).toBe('Failed');
    expect(rows[2]!.text()).toContain('This file type isn\'t supported.');
  });

  it('announces each finished or failed file once, in one polite live region', async () => {
    const w = mount(FileUpload, { props: { ...labels, accept: '', maxBytes, items: items.slice(0, 1) } });
    const live = w.get('[aria-live="polite"]');
    expect(live.classes()).toContain('sr-only');
    expect(live.findAll('li')).toHaveLength(0);
    await w.setProps({ items: [{ ...items[0]!, state: 'done', progress: 1 }, items[2]!] });
    expect(live.findAll('li').map(l => l.text())).toEqual(['refund-policy.pdf: Uploaded', 'photo.png: Failed. This file type isn\'t supported.']);
    expect(w.findAll('[aria-live]')).toHaveLength(1);
  });

  it('refuses a dropped file of the wrong type, by extension or MIME, with its reason', async () => {
    const w = mount(FileUpload, { props: { ...labels, accept: '.pdf,.md,text/plain', maxBytes: 100, items: [] } });
    const png = new File(['x'], 'photo.png', { type: 'image/png' });
    const txt = new File(['x'], 'notes', { type: 'text/plain' });
    const pdf = new File(['x'], 'Policy.PDF');
    await w.get('[data-dropzone]').trigger('drop', { dataTransfer: { files: [png, txt, pdf] } });
    expect(w.emitted('select')![0]![0]).toEqual([txt, pdf]);
    expect(w.emitted('reject')![0]![0]).toEqual([{ file: png, reason: 'type' }]);
  });

  it('refuses a dropped or picked file over the max size', async () => {
    const w = mount(FileUpload, { props: { ...labels, accept: '.txt', maxBytes: 3, items: [] } });
    const big = new File(['abcd'], 'big.txt');
    const ok = new File(['abc'], 'ok.txt');
    await w.get('[data-dropzone]').trigger('drop', { dataTransfer: { files: [big, ok] } });
    expect(w.emitted('select')![0]![0]).toEqual([ok]);
    expect(w.emitted('reject')![0]![0]).toEqual([{ file: big, reason: 'size' }]);
    const input = w.get<HTMLInputElement>('input[type="file"]');
    Object.defineProperty(input.element, 'files', { value: [big], configurable: true });
    await input.trigger('change');
    expect(w.emitted('select')).toHaveLength(1);
    expect(w.emitted('reject')![1]![0]).toEqual([{ file: big, reason: 'size' }]);
  });
});
