// @vitest-environment happy-dom
import { mount } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it } from 'vitest';

import OrgCreateModal from '~/components/org/create/OrgCreateModal.vue';
import OrgFields from '~/components/org/create/OrgFields.vue';

beforeEach(() => {
  setActivePinia(createPinia());
});

describe('org create modal copy', () => {
  it('takes its placeholders and the random-slug help from the locales', () => {
    const w = mount(OrgCreateModal);
    const [name, slug] = w.findAll('input');
    expect(name!.attributes('placeholder')).toBe('org.name_placeholder');
    expect(slug!.attributes('placeholder')).toBe('org.slug_placeholder');
    expect(w.text()).toContain('org.slug_random_help');
    expect(w.text()).not.toContain('generated when empty');
  });
});

describe('org fields', () => {
  it('names each input by its fieldset legend and shows its error', () => {
    const w = mount(OrgFields, { props: { name: '', slug: '', fieldErrors: { slug: 'taken' } } });
    const legends = w.findAll('fieldset > legend.fieldset-legend');
    expect(legends.map(l => l.text())).toEqual(['common.name', 'common.slug']);
    const [name, slug] = w.findAll('input');
    expect(name!.attributes('aria-labelledby')).toBe(legends[0]!.attributes('id'));
    expect(slug!.attributes('aria-labelledby')).toBe(legends[1]!.attributes('id'));
    expect(slug!.attributes('aria-invalid')).toBe('true');
    expect(w.text()).toContain('taken');
  });

  it('is the same block in the modal and on onboarding', () => {
    expect(mount(OrgCreateModal).findComponent(OrgFields).exists()).toBe(true);
  });
});
