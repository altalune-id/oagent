// @vitest-environment happy-dom
import { mount } from '@vue/test-utils';
import { describe, expect, it, vi } from 'vitest';
import { ref } from 'vue';
import ConfirmModal from './ConfirmModal.vue';

function handle() {
  return {
    target: ref<unknown>(null),
    busy: ref(false),
    error: ref<{ message: string } | null>(null),
    cancel: vi.fn(),
    confirm: vi.fn(async () => true),
  };
}

const props = { title: 'Revoke this token?', body: 'Anything using cli stops working.', confirmLabel: 'Revoke', cancelLabel: 'Cancel' };

describe('confirmModal', () => {
  it('opens only while the handle has a target, titled and with its body', async () => {
    const h = handle();
    const w = mount(ConfirmModal, { props: { ...props, handle: h }, attachTo: document.body });
    expect(w.get('dialog').element.open).toBe(false);
    h.target.value = { id: 'K9tWq4ZbN2xLm7VcR5pHs' };
    await w.vm.$nextTick();
    const dialog = w.get('dialog');
    expect(dialog.element.open).toBe(true);
    expect(document.getElementById(dialog.attributes('aria-labelledby')!)?.textContent?.trim()).toBe('Revoke this token?');
    expect(dialog.text()).toContain('Anything using cli stops working.');
    w.unmount();
  });

  it('cancels and confirms through the handle', async () => {
    const h = handle();
    h.target.value = {};
    const w = mount(ConfirmModal, { props: { ...props, handle: h } });
    const [cancel, confirm] = w.findAll('.modal-action button');
    await cancel!.trigger('click');
    await confirm!.trigger('click');
    expect(h.cancel).toHaveBeenCalledTimes(1);
    expect(h.confirm).toHaveBeenCalledTimes(1);
  });

  it('disables both buttons and shows a hidden spinner while busy, and shows a refusal', async () => {
    const h = handle();
    h.target.value = {};
    const w = mount(ConfirmModal, { props: { ...props, handle: h } });
    expect(w.find('.loading').exists()).toBe(false);
    h.busy.value = true;
    h.error.value = { message: 'Couldn\'t revoke the token.' };
    await w.vm.$nextTick();
    expect(w.findAll('.modal-action button').every(b => b.attributes('disabled') !== undefined)).toBe(true);
    expect(w.get('.modal-action .loading').attributes('aria-hidden')).toBe('true');
    expect(w.get('[role="alert"]').text()).toContain('Couldn\'t revoke the token.');
  });
});
