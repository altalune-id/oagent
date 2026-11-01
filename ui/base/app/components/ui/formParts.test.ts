import type { Component } from 'vue';
// @vitest-environment happy-dom
import { mount } from '@vue/test-utils';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { h } from 'vue';
import AppDataTable from './DataTable.vue';
import FormField from './FormField.vue';
import SecretReveal from './SecretReveal.vue';
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
    expect(w.findAll('tbody td')[1]!.classes()).toContain('tabular-nums');
    expect(w.find('ul.list').exists()).toBe(false);
  });

  it('shows skeleton rows while loading, busy for assistive tech', () => {
    const w = mount(DataTable, { props: { columns, rows: [], rowKey: 'id', caption: 'Agents', loading: true }, slots: { 'empty': '<p>none</p>', 'row-sm': '<span>row</span>' } });
    expect(w.get('table').attributes('aria-busy')).toBe('true');
    expect(w.findAll('table .skeleton')).toHaveLength(6);
    expect(w.findAll('ul.list .skeleton')).toHaveLength(3);
    expect(w.text()).not.toContain('none');
  });

  it('shows the empty slot when done with no rows, and a stacked list on phones', () => {
    expect(mount(DataTable, { props: { columns, rows: [], rowKey: 'id', caption: 'Agents' }, slots: { empty: '<p>none</p>' } }).text()).toBe('none');
    const w = mount(DataTable, { props: { columns, rows, rowKey: 'id', caption: 'Agents' }, slots: { 'row-sm': ({ row }: { row: { name: string } }) => h('span', row.name) } });
    expect(w.get('ul.list').attributes('aria-label')).toBe('Agents');
    expect(w.findAll('li.list-row').map(li => li.text())).toEqual(['Refund Triage', 'Order Status']);
    expect(w.get('.overflow-x-auto').classes()).toContain('max-sm:hidden');
  });
});

describe('formField', () => {
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

const labels = { once: 'Shown once', copy: 'Copy secret', copyEnv: 'Copy as .env', confirm: 'I stored it', missing: 'Could not load' };

describe('secretReveal', () => {
  it('warns it is shown once, offers copy and .env copy, and reports the stored tick', async () => {
    const w = mount(SecretReveal, { props: { value: 's3cret', labels, env: { A: '1', B: 's3cret' } } });
    expect(w.get('[role="note"]').text()).toBe('Shown once');
    expect(w.get('code').text()).toBe('s3cret');
    expect(w.findAll('button').map(b => b.attributes('aria-label'))).toEqual(['Copy secret', 'Copy as .env']);
    await w.get('input[type="checkbox"]').setValue(true);
    expect(w.emitted('update:confirmed')).toEqual([[true]]);
  });

  it('shows an error with a retry slot when the secret is missing', () => {
    const w = mount(SecretReveal, { props: { value: '', labels }, slots: { retry: '<button>Rotate</button>' } });
    expect(w.get('[role="alert"]').text()).toContain('Could not load');
    expect(w.text()).toContain('Rotate');
    expect(w.find('code').exists()).toBe(false);
  });

  it('skips the .env copy without env values', () => {
    expect(mount(SecretReveal, { props: { value: 'x', labels } }).findAll('button')).toHaveLength(1);
  });
});

describe('toast', () => {
  afterEach(() => {
    const { toasts, dismiss } = useToast();
    [...toasts].forEach(t => dismiss(t.id));
    vi.useRealTimers();
  });

  it('announces status politely and errors as alerts, and hides after its lifetime', async () => {
    vi.useFakeTimers();
    const { push } = useToast();
    const w = mount(ToastHost, { props: { closeLabel: 'Close' } });
    push('success', 'Saved');
    push('error', 'Failed');
    await w.vm.$nextTick();
    expect(w.attributes('aria-live')).toBe('polite');
    expect(w.findAll('.alert').map(a => [a.attributes('role'), a.text()])).toEqual([['status', 'Saved'], ['alert', 'Failed']]);
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
