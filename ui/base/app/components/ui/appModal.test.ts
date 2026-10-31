// @vitest-environment happy-dom
import { mount } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import AppModal from './AppModal.vue';

function render(props: Record<string, unknown>) {
  return mount(AppModal, { props, slots: { default: '<p>Body</p>', footer: '<button>Done</button>' }, attachTo: document.body });
}

describe('appModal', () => {
  it('opens as a native modal dialog named by its title', async () => {
    const w = render({ open: false, title: 'New organization' });
    const dialog = w.get('dialog');
    expect(dialog.element.open).toBe(false);
    await w.setProps({ open: true });
    expect(dialog.element.open).toBe(true);
    expect(dialog.classes()).not.toContain('modal-open');
    expect(document.getElementById(dialog.attributes('aria-labelledby')!)!.textContent!.trim()).toBe('New organization');
    await w.setProps({ open: false });
    expect(dialog.element.open).toBe(false);
  });

  it('closes on Esc and on the backdrop while dismissible', async () => {
    const w = render({ open: true, title: 'Rotate' });
    await w.get('dialog').trigger('cancel');
    await w.get('form.modal-backdrop').trigger('submit');
    expect(w.emitted('close')).toHaveLength(2);
  });

  it('ignores Esc and has no backdrop until the footer action runs, when not dismissible', async () => {
    const w = render({ open: true, title: 'Save the client secret', dismissible: false });
    await w.get('dialog').trigger('cancel');
    expect(w.emitted('close')).toBeUndefined();
    expect(w.find('form.modal-backdrop').exists()).toBe(false);
  });

  it('reopens when the browser closes it while open is still true, so a busy or locked modal stays in step', async () => {
    const w = render({ open: true, title: 'Rotate', dismissible: false });
    const dialog = w.get('dialog').element as HTMLDialogElement;
    dialog.close();
    await w.vm.$nextTick();
    expect(dialog.open).toBe(true);
    await w.setProps({ open: false });
    expect(dialog.open).toBe(false);
  });

  it('has no name without a title', () => {
    expect(render({ open: true }).get('dialog').attributes('aria-labelledby')).toBeUndefined();
  });
});
