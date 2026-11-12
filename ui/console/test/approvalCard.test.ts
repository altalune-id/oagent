// @vitest-environment happy-dom
import { flushPromises, mount } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import ApprovalCard from '~/components/approval/ApprovalCard.vue';
import { kopiApproval } from '~/preview/samples';

const approval = kopiApproval();
const button = (w: ReturnType<typeof mount>, text: string) => w.findAll('button').find(b => b.text().includes(text))!;

describe('approval card', () => {
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
