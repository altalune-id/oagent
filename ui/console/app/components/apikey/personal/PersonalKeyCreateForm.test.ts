// @vitest-environment happy-dom
import { mount } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import PersonalKeyCreateForm from './PersonalKeyCreateForm.vue';

const props = {
  expiryOptions: [{ days: 7, label: '7 days' }, { days: 30, label: '30 days' }],
  projects: [{ id: 'x6MH9WPWjktMwCBKHy4pF', name: 'Web' }],
  scopeOptions: [{ name: 'agent:read', label: 'Read agents' }],
  allProjects: false,
};

describe('the personal key create form', () => {
  it('labels every text input and select through a matching label', () => {
    const w = mount(PersonalKeyCreateForm, { props });
    const controls = w.findAll('input[type="text"], select');
    expect(controls).toHaveLength(2);
    for (const c of controls) {
      expect(w.find(`label[for="${c.attributes('id')}"]`).exists(), c.html()).toBe(true);
    }
  });

  it('labels each checkbox and radio by wrapping it in its label', () => {
    const w = mount(PersonalKeyCreateForm, { props });
    const choices = w.findAll('input[type="checkbox"], input[type="radio"]');
    expect(choices.length).toBe(4);
    expect(choices.every(c => c.element.closest('label') !== null)).toBe(true);
  });

  it('submits only through the enabled button', async () => {
    const w = mount(PersonalKeyCreateForm, { props: { ...props, canSubmit: true } });
    expect(w.get('button[type="submit"]').attributes('disabled')).toBeUndefined();
    await w.get('form').trigger('submit');
    expect(w.emitted('submit')).toHaveLength(1);
    const off = mount(PersonalKeyCreateForm, { props });
    expect(off.get('button[type="submit"]').attributes('disabled')).toBeDefined();
  });
});
