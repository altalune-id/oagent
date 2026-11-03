import type { Component } from 'vue';
// @vitest-environment happy-dom
import { flushPromises, mount } from '@vue/test-utils';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { h } from 'vue';
import { copiedLabelKey } from './copy';
import CopyButton from './CopyButton.vue';
import AppDataTable from './DataTable.vue';
import FormField from './FormField.vue';
import SearchInput from './SearchInput.vue';
import { toastLifetimeMs, useToast } from './toast';
import ToastHost from './ToastHost.vue';

const DataTable: Component = AppDataTable;
const columns = [{ key: 'name', label: 'Agent' }, { key: 'runs', label: 'Runs', align: 'end' as const }];
const rows = [{ id: 'a1', name: 'Refund Triage', runs: 12 }, { id: 'a2', name: 'Order Status', runs: 3 }];

describe('dataTable', () => {
  it('renders a captioned table with column headers, cell slots and right-aligned numbers', () => {
    const w = mount(DataTable, {
      props: { columns, rows, rowKey: 'id', caption: 'Agents' },
      slots: { 'cell-name': ({ row }: { row: { name: string } }) => h('b', row.name) },
    });
    expect(w.get('caption').text()).toBe('Agents');
    expect(w.findAll('th').map(t => [t.text(), t.attributes('scope')])).toEqual([['Agent', 'col'], ['Runs', 'col']]);
    expect(w.findAll('tbody b').map(b => b.text())).toEqual(['Refund Triage', 'Order Status']);
    expect(w.findAll('tbody td')[1]!.classes()).toEqual(['text-end', 'tabular-nums']);
    expect(w.find('ul.list').exists()).toBe(false);
  });

  it('draws the design table: dense text, muted shaded headers, mono cells and a hidden label for the menu column', () => {
    const cols = [{ key: 'model', label: 'Model', mono: true }, { key: 'menu', label: 'Actions', hideLabel: true }];
    const w = mount(DataTable, { props: { columns: cols, rows, rowKey: 'id', caption: 'Agents' } });
    expect(w.get('table').classes()).toEqual(['table', 'table-sm', 'text-ui']);
    expect(w.get('thead').classes()).toEqual(['text-base-content/70']);
    expect(w.get('th').classes()).toEqual(['bg-base-200', 'text-xs', 'font-medium']);
    expect(w.findAll('th span').map(s => s.classes())).toEqual([[], ['sr-only']]);
    expect(w.get('tbody td').classes()).toEqual(['font-mono', 'text-xs']);
    expect(w.get('tbody tr').classes()).toEqual([]);
    expect(w.find('th[aria-sort]').exists()).toBe(false);
  });

  it('sorts through a header button that announces its order, flipping on a second press', async () => {
    const cols = [{ key: 'name', label: 'Agent', sortable: true }, { key: 'runs', label: 'Runs' }];
    const w = mount(DataTable, { props: { 'columns': cols, rows, 'rowKey': 'id', 'caption': 'Agents', 'sort': undefined, 'onUpdate:sort': (v: unknown) => w.setProps({ sort: v }) } });
    const [name, runs] = w.findAll('th');
    expect(name!.attributes('aria-sort')).toBe('none');
    expect(runs!.attributes('aria-sort')).toBeUndefined();
    const button = name!.get('button');
    expect(button.attributes('type')).toBe('button');
    expect(button.text()).toBe('Agent');
    expect(button.get('[data-icon="lucide:arrow-down-up"]').attributes('aria-hidden')).toBe('true');
    await button.trigger('click');
    expect(w.emitted('update:sort')![0]).toEqual([{ key: 'name', dir: 'ascending' }]);
    expect(w.get('th').attributes('aria-sort')).toBe('ascending');
    await button.trigger('click');
    expect(w.get('th').attributes('aria-sort')).toBe('descending');
  });

  it('shows skeleton rows while loading, busy for assistive tech', () => {
    const w = mount(DataTable, { props: { columns, rows: [], rowKey: 'id', caption: 'Agents', loading: true }, slots: { empty: '<p>none</p>' } });
    expect(w.get('table').attributes('aria-busy')).toBe('true');
    expect(w.findAll('table .skeleton')).toHaveLength(6);
    expect(w.text()).not.toContain('none');
  });

  it('shows the empty slot when done with no rows, and keeps the table in its scroll box on phones', () => {
    expect(mount(DataTable, { props: { columns, rows: [], rowKey: 'id', caption: 'Agents' }, slots: { empty: '<p>none</p>' } }).text()).toBe('none');
    const w = mount(DataTable, { props: { columns, rows, rowKey: 'id', caption: 'Agents' } });
    expect(w.get('.overflow-x-auto').classes()).not.toContain('max-sm:hidden');
    expect(w.find('ul').exists()).toBe(false);
  });
});

describe('formField', () => {
  it('puts the design label, semibold, above the control and a muted hint below it', () => {
    const w = mount(FormField, { props: { label: 'Model', hint: 'About $0.002 per step at this size' }, slots: { default: '<select></select>' } });
    expect(w.classes()).toEqual(['flex', 'min-w-0', 'flex-col', 'gap-1']);
    expect(w.get('label').classes()).toEqual(['label', 'text-ui', 'font-semibold', 'text-base-content']);
    expect(w.get('p').classes()).toEqual(['label', 'whitespace-normal', 'text-xs', 'text-base-content/70']);
  });

  it('draws the error message in the design error text, 12px with normal line height, 8px below the control', () => {
    const w = mount(FormField, { props: { label: 'Max steps', error: 'Enter at least 1 step' }, slots: { default: '<input>' } });
    expect(w.get(`#${w.get('label').attributes('for')}-error`).classes()).toEqual(['mt-2', 'flex', 'items-start', 'gap-1', 'text-xs', 'leading-[normal]', 'text-error']);
  });

  it('labels the control and describes it by the hint and the error', () => {
    const w = mount(FormField, {
      props: { label: 'Name', hint: 'Shown on runs', error: 'Too long', required: true, requiredLabel: 'required' },
      slots: { default: ({ id, describedBy, invalid }: { id: string, describedBy?: string, invalid: boolean }) => h('input', { 'id': id, 'aria-describedby': describedBy, 'aria-invalid': invalid }) },
    });
    const input = w.get('input');
    expect(w.get('label').attributes('for')).toBe(input.attributes('id'));
    expect(w.get('label').text()).toContain('(required)');
    const ids = input.attributes('aria-describedby')!.split(' ');
    expect(ids.map(id => w.get(`#${id}`).text())).toEqual(['Shown on runs', 'Too long']);
    expect(input.attributes('aria-invalid')).toBe('true');
  });

  it('describes nothing without hint or error', () => {
    const w = mount(FormField, { props: { label: 'Name' }, slots: { default: ({ describedBy }: { describedBy?: string }) => h('input', { 'aria-describedby': describedBy }) } });
    expect(w.get('input').attributes('aria-describedby')).toBeUndefined();
  });
});

describe('toast', () => {
  afterEach(() => {
    const { toasts, dismiss } = useToast();
    [...toasts].forEach(t => dismiss(t.id));
    vi.useRealTimers();
  });

  it('announces every toast through one polite live region on the host, and hides it after its lifetime', async () => {
    vi.useFakeTimers();
    const { push } = useToast();
    const w = mount(ToastHost, { props: { closeLabel: 'Close' } });
    push('success', 'Saved');
    push('error', 'Failed');
    await w.vm.$nextTick();
    expect([w.attributes('role'), w.attributes('aria-live')]).toEqual(['status', 'polite']);
    expect(w.findAll('.alert').map(a => [a.attributes('role'), a.attributes('aria-live')])).toEqual([[undefined, undefined], [undefined, undefined]]);
    expect(w.findAll('.alert').map(a => a.classes())).toEqual([['alert', 'text-ui', 'shadow-sm'], ['alert', 'text-ui', 'shadow-sm']]);
    expect(w.findAll('.alert').map(a => [a.get('[data-icon]').attributes('data-icon'), a.text()])).toEqual([['lucide:circle-check', 'Saved'], ['lucide:circle-alert', 'Failed']]);
    vi.advanceTimersByTime(toastLifetimeMs);
    await w.vm.$nextTick();
    expect(w.findAll('.alert')).toHaveLength(0);
  });

  it('pauses while hovered or focused and closes from its button', async () => {
    vi.useFakeTimers();
    const { push } = useToast();
    const w = mount(ToastHost);
    push('info', 'Copied');
    await w.vm.$nextTick();
    await w.get('.alert').trigger('mouseenter');
    vi.advanceTimersByTime(toastLifetimeMs * 2);
    await w.vm.$nextTick();
    expect(w.findAll('.alert')).toHaveLength(1);
    await w.get('.alert').trigger('mouseleave');
    await w.get('.alert').trigger('focusin');
    await w.get('.alert').trigger('focusout');
    await w.get('button[aria-label="Dismiss"]').trigger('click');
    expect(w.findAll('.alert')).toHaveLength(0);
  });
});

describe('searchInput', () => {
  it('wraps a named search field and its hidden icon in the small input, and binds the query', async () => {
    const w = mount(SearchInput, { props: { 'label': 'Filter agents', 'modelValue': '', 'onUpdate:modelValue': (v: string) => w.setProps({ modelValue: v }) } });
    expect(w.classes()).toEqual(['input', 'input-sm', 'text-ui']);
    expect(w.get('.sr-only').text()).toBe('Filter agents');
    expect(w.get('[data-icon="lucide:search"]').attributes('aria-hidden')).toBe('true');
    const input = w.get('input[type="search"]');
    expect(input.attributes('placeholder')).toBe('Filter agents');
    await input.setValue('refund');
    expect(w.props('modelValue')).toBe('refund');
  });
});

describe('copyButton', () => {
  it('toasts the given label, else the app copied label, else Copied', async () => {
    vi.stubGlobal('navigator', { clipboard: { writeText: vi.fn().mockResolvedValue(undefined) } });
    const { toasts } = useToast();
    toasts.splice(0);
    await mount(CopyButton, { props: { value: 'x', text: 'Copy issuer', copiedLabel: 'Copied issuer' } }).get('button').trigger('click');
    await flushPromises();
    expect(toasts.map(t => t.text)).toEqual(['Copied issuer']);
    toasts.splice(0);
    await mount(CopyButton, { props: { value: 'x' }, global: { provide: { [copiedLabelKey as symbol]: () => 'Disalin' } } }).get('button').trigger('click');
    await flushPromises();
    expect(toasts.map(t => t.text)).toEqual(['Disalin']);
    toasts.splice(0);
    await mount(CopyButton, { props: { value: 'x' } }).get('button').trigger('click');
    await flushPromises();
    expect(toasts.map(t => t.text)).toEqual(['Copied']);
    vi.unstubAllGlobals();
  });
});
