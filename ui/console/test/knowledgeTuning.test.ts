// @vitest-environment happy-dom
import type { KnowledgeTuning } from '~/components/agent/configure/agentConfig';
import type { RetrievalDefaults } from '~/stores/knowledge';
import { flushPromises, mount } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import { defineComponent, h, ref } from 'vue';
import { setKnownKeys, setLocale } from '#test/nuxt-globals';
import KnowledgeTuningForm from '~/components/agent/configure/KnowledgeTuning.vue';

const defaults: RetrievalDefaults = {
  topK: 6,
  minScore: 0.25,
  maxTokens: 3000,
  ranges: { topK: [1, 10], minScore: [0, 1], maxTokens: [500, 8000] },
  presets: [{ name: 'precise', topK: 3, minScore: 0.35 }, { name: 'balanced', topK: 6, minScore: 0.25 }, { name: 'broad', topK: 8, minScore: 0.1 }],
};

function host(start: KnowledgeTuning = { topK: 0, maxTokens: 0 }, off = false) {
  const tuning = ref<KnowledgeTuning>(start);
  const w = mount(defineComponent({
    setup: () => () => h(KnowledgeTuningForm, { 'defaults': defaults, off, 'modelValue': tuning.value, 'onUpdate:modelValue': (v: KnowledgeTuning) => (tuning.value = v) }),
  }), { attachTo: document.body });
  const inputs = () => w.findAll('input[type="number"]');
  return { w, tuning, inputs };
}

async function type(el: ReturnType<ReturnType<typeof host>['inputs']>[number], text: string) {
  for (let i = 1; i <= text.length; i++) {
    await el.setValue(text.slice(0, i));
    await flushPromises();
    expect((el.element as HTMLInputElement).value, 'the field keeps what was typed').toBe(text.slice(0, i));
  }
}

describe('knowledge tuning form', () => {
  it('keeps what you type: 0.05 commits 0.05, character by character', async () => {
    const { tuning, inputs } = host();
    await type(inputs()[1]!, '0.05');
    expect(tuning.value.minScore).toBe(0.05);
    expect((inputs()[1]!.element as HTMLInputElement).value).toBe('0.05');
  });

  it('refuses a typed 0 in passages or budget with the range, and clearing means recommended', async () => {
    const { w, tuning, inputs } = host({ topK: 4, maxTokens: 900 });
    await inputs()[0]!.setValue('0');
    expect(tuning.value.topK).toBe(4);
    expect(w.text()).toContain('configure.tuning_range|{"min":1,"max":10}');
    await inputs()[0]!.setValue('');
    expect(tuning.value.topK).toBe(0);
    expect(w.text()).not.toContain('configure.tuning_range');
  });

  it('shows the range for input the browser can\'t read as a number, and keeps it', async () => {
    const { w, tuning, inputs } = host({ topK: 4, maxTokens: 0 });
    const el = inputs()[0]!.element as HTMLInputElement;
    Object.defineProperty(el, 'validity', { value: { badInput: true }, configurable: true });
    el.value = '';
    await inputs()[0]!.trigger('input');
    expect(tuning.value.topK).toBe(4);
    expect(w.text()).toContain('configure.tuning_range|{"min":1,"max":10}');
  });

  it('drops a stale error when the tuning changes from outside, and keeps one while you edit another field', async () => {
    const { w, tuning, inputs } = host({ topK: 4, maxTokens: 0 });
    await inputs()[0]!.setValue('11');
    await inputs()[2]!.setValue('900');
    await flushPromises();
    expect(w.text(), 'editing another field keeps the first error').toContain('configure.tuning_range|{"min":1,"max":10}');
    tuning.value = { topK: 5, maxTokens: 0 };
    await flushPromises();
    expect(w.text()).not.toContain('configure.tuning_range');
    expect(inputs().map(i => (i.element as HTMLInputElement).value)).toEqual(['5', '', '']);
    const el = inputs()[1]!.element as HTMLInputElement;
    Object.defineProperty(el, 'validity', { value: { badInput: true }, configurable: true });
    el.value = '';
    await inputs()[1]!.trigger('input');
    expect(w.text()).toContain('configure.tuning_range|{"min":0,"max":1}');
    tuning.value = { topK: 6, maxTokens: 0 };
    await flushPromises();
    expect(w.text(), 'a reload clears an error even when the field already shows the value').not.toContain('configure.tuning_range');
  });

  it('labels the preset group, announces Custom, and lets a fieldset disable the chips', async () => {
    const { w } = host({ topK: 7, maxTokens: 0 });
    const group = w.get('[role="radiogroup"]');
    expect(w.get(`#${group.attributes('aria-labelledby')}`).text()).toBe('configure.tuning_preset');
    expect(w.get(`#${group.attributes('aria-describedby')}`).text()).toBe('configure.preset_custom');
    expect(w.findAll('[role="radiogroup"] label').every(l => l.classes().includes('has-[:disabled]:btn-disabled') && l.classes().includes('has-[:checked]:btn-primary'))).toBe(true);
  });

  it('explains the selected preset, and keeps the fields under a closed Advanced unless the values are custom', async () => {
    setKnownKeys('configure.preset_balanced_hint', 'configure.preset_precise_hint', 'configure.preset_custom_hint');
    const { w, tuning } = host();
    expect(w.get('[data-preset-hint]').text()).toBe('configure.preset_balanced_hint');
    const details = () => w.get('details').element as HTMLDetailsElement;
    expect(details().open).toBe(false);
    tuning.value = { topK: 3, minScore: 0.35, maxTokens: 0 };
    await flushPromises();
    expect(w.get('[data-preset-hint]').text()).toBe('configure.preset_precise_hint');
    tuning.value = { topK: 7, maxTokens: 0 };
    await flushPromises();
    expect(details().open).toBe(true);
    expect(w.get('[data-preset-hint]').text()).toBe('configure.preset_custom_hint');
    setKnownKeys('configure.preset_custom_hint');
    tuning.value = { topK: 8, minScore: 0.1, maxTokens: 0 };
    await flushPromises();
    expect(w.find('[data-preset-hint]').exists(), 'a preset with no hint key shows no line').toBe(false);
    setKnownKeys();
  });

  it('formats the default hints in the UI locale', async () => {
    await setLocale('id-ID');
    const { w } = host();
    expect(w.text()).toContain('configure.tuning_recommended|{"n":"0,25"}');
    expect(w.text()).toContain('configure.tuning_recommended|{"n":"3.000"}');
    await setLocale('en-US');
  });

  it('disables Use recommended while the values already are the recommended ones', async () => {
    const { w, tuning } = host();
    const reset = () => w.findAll('button').find(b => b.text().includes('configure.tuning_reset'))!;
    expect(reset().attributes('disabled')).toBeDefined();
    tuning.value = { topK: 3, maxTokens: 0 };
    await flushPromises();
    expect(reset().attributes('disabled')).toBeUndefined();
  });

  it('puts a range error before the default hint', async () => {
    const { inputs } = host({ topK: 7, maxTokens: 0 });
    await inputs()[0]!.setValue('11');
    const box = inputs()[0]!.element.closest('label')!.parentElement!;
    const texts = [...box.children].map(c => c.textContent!.trim()).filter(Boolean);
    expect(texts.findIndex(x => x.startsWith('configure.tuning_range'))).toBeLessThan(texts.findIndex(x => x.startsWith('configure.tuning_recommended')));
  });
});
