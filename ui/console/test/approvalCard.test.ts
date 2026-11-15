// @vitest-environment happy-dom
import { flushPromises, mount } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import ApprovalCard from '~/components/approval/ApprovalCard.vue';
import { kopiApproval } from '~/preview/samples';

const approval = kopiApproval();
const button = (w: ReturnType<typeof mount>, text: string) => w.findAll('button').find(b => b.text().includes(text))!;

describe('approval card', () => {
  it('labels a built-in web tool call heading and never relabels an argument line', () => {
    const http = { ...approval, tool: 'http_request', title: 'web_tools · http_request', args: [{ label: 'method', value: 'POST' }, { label: 'url', value: 'https://api.example.com/v1/orders' }, { label: 'web_tools · x', value: 'web_tools · y' }], raw: { method: 'POST', url: 'https://api.example.com/v1/orders' } };
    const w = mount(ApprovalCard, { props: { approval: http } });
    expect(w.get('h3').text()).toContain('approvalcard.web_tools · http_request');
    expect(w.findAll('dt').map(d => d.text())).toEqual(['method', 'url', 'web_tools · x']);
    expect(w.findAll('dd').map(d => d.text())).toContain('web_tools · y');
    expect(w.text()).not.toMatch(/secret|value_sealed/i);
  });

  it('labels each call heading of a batched request', () => {
    const batch = { ...approval, calls: [{ title: 'web_tools · http_request', args: [] }, { title: 'yasaku · record_expense', args: [] }] };
    const w = mount(ApprovalCard, { props: { approval: batch } });
    expect(w.findAll('h4').map(h => h.text())).toEqual(['approvalcard.web_tools · http_request', 'yasaku · record_expense']);
  });

  it('hides the gating argument from the details but keeps it in the raw arguments', () => {
    const w = mount(ApprovalCard, { props: { approval } });
    expect(w.findAll('dt').map(d => d.text())).toEqual(['Jumlah', 'Kategori', 'Dompet', 'Tanggal']);
    expect(w.get('pre').text()).toContain('"confirm": true');
  });

  it('focuses the reason on Reject, returns to Reject on Cancel, and focuses the decision afterwards', async () => {
    const w = mount(ApprovalCard, { props: { approval }, attachTo: document.body });
    await button(w, 'approvalcard.reject').trigger('click');
    await flushPromises();
    expect(document.activeElement).toBe(w.get('input').element);
    await button(w, 'common.cancel').trigger('click');
    await flushPromises();
    expect(document.activeElement).toBe(button(w, 'approvalcard.reject').element);
    await button(w, 'approvalcard.approve_run').trigger('click');
    expect(w.emitted('approve')).toHaveLength(1);
    await w.setProps({ approval: { ...approval, status: 'approved', decidedBy: 'you', decidedAt: new Date() } });
    await flushPromises();
    expect(document.activeElement).toBe(w.get('[role="status"]').element);
    expect(w.get('[role="status"]').attributes('tabindex')).toBe('-1');
    w.unmount();
  });

  it('draws the warning border only while the request is pending', async () => {
    const w = mount(ApprovalCard, { props: { approval } });
    expect(w.get('article').classes()).toContain('border-warning');
    await w.setProps({ approval: { ...approval, status: 'approved', decidedBy: 'Sari', decidedAt: new Date() } });
    expect(w.get('article').classes()).toEqual(expect.arrayContaining(['border-base-300']));
    expect(w.get('article').classes()).not.toContain('border-warning');
  });

  it('does not take focus for a decision made elsewhere', async () => {
    const w = mount(ApprovalCard, { props: { approval }, attachTo: document.body });
    await w.setProps({ approval: { ...approval, status: 'rejected', decidedBy: 'Sari' } });
    await flushPromises();
    expect(document.activeElement).toBe(document.body);
    w.unmount();
  });
});
