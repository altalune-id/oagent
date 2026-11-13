<script setup lang="ts">
import type { KnowledgeTuning } from './agentConfig';
import type { TuningField } from './tuning';
import type { RetrievalDefaults } from '~/stores/knowledge';
import AppButton from '#base/components/ui/AppButton.vue';
import FieldError from '#base/components/ui/FieldError.vue';
import { presetLabel, presetOf, recommendedTuning, withField, withPreset } from './tuning';

const { defaults, off } = defineProps<{ defaults: RetrievalDefaults, off: boolean }>();
const tuning = defineModel<KnowledgeTuning>({ required: true });

const { t, te, locale } = useI18n();
const uid = useId();
const fields: TuningField[] = ['topK', 'minScore', 'maxTokens'];
const labels: Record<TuningField, string> = { topK: 'configure.tuning_top_k', minScore: 'configure.tuning_min_score', maxTokens: 'configure.tuning_max_tokens' };
const steps: Record<TuningField, string> = { topK: '1', minScore: '0.05', maxTokens: '100' };

function textOf(v: KnowledgeTuning, f: TuningField) {
  const n = v[f];
  return n === undefined || (f !== 'minScore' && n === 0) ? '' : String(n);
}
const texts = ref<Record<TuningField, string>>({ topK: '', minScore: '', maxTokens: '' });
const errors = ref<Partial<Record<TuningField, [number, number]>>>({});

let ownWrite = '';

// shows reports whether a field's text already stands for a value, so a re-render never rewrites what is being typed.
function shows(text: string, value: string) {
  return text.trim() === '' ? value === '' : value !== '' && Number(text) === Number(value);
}

watch(tuning, (v) => {
  const own = JSON.stringify(v) === ownWrite;
  ownWrite = '';
  for (const f of fields) {
    const want = textOf(v, f);
    if (own ? !errors.value[f] && !shows(texts.value[f], want) : !shows(texts.value[f], want) || errors.value[f]) {
      texts.value[f] = want;
      const { [f]: _, ...rest } = errors.value;
      errors.value = rest;
    }
  }
}, { immediate: true, deep: true });

const preset = computed(() => presetOf(tuning.value, defaults));
const presetHint = computed(() => {
  const key = `configure.preset_${preset.value ?? 'custom'}_hint`;
  return te(key) ? t(key) : '';
});
const atRecommended = computed(() => tuning.value.topK === 0 && tuning.value.maxTokens === 0 && tuning.value.minScore === undefined && !Object.keys(errors.value).length);
const recommended = computed(() => {
  const n = (v: number, digits = 0) => new Intl.NumberFormat(locale.value, { minimumFractionDigits: digits, maximumFractionDigits: Math.max(digits, 2) }).format(v);
  return { topK: n(defaults.topK), minScore: n(defaults.minScore, 2), maxTokens: n(defaults.maxTokens) };
});
const advanced = ref(preset.value === null);
watch(() => preset.value === null || Object.keys(errors.value).length > 0, (custom) => {
  if (custom) {
    advanced.value = true;
  }
});

function edit(f: TuningField, el: HTMLInputElement) {
  texts.value[f] = el.value;
  const res = el.validity?.badInput ? { error: defaults.ranges[f] } : withField(tuning.value, f, el.value, defaults);
  const { [f]: _, ...rest } = errors.value;
  errors.value = res.error ? { ...rest, [f]: res.error } : rest;
  if (res.tuning) {
    ownWrite = JSON.stringify(res.tuning);
    tuning.value = res.tuning;
  }
}

function pick(name: string) {
  const p = defaults.presets.find(x => x.name === name);
  if (p) {
    errors.value = {};
    tuning.value = withPreset(tuning.value, p, defaults);
  }
}

function reset() {
  errors.value = {};
  tuning.value = recommendedTuning();
}
</script>

<template>
  <div data-tuning class="flex flex-col gap-3 rounded-box border border-base-300 p-3">
    <div class="flex flex-wrap items-center justify-between gap-2">
      <span :id="`${uid}-title`" class="text-ui font-semibold">{{ t('configure.tuning_title') }}</span>
      <AppButton variant="ghost" size="sm" :disabled="off || atRecommended" @click="reset()">
        <Icon name="lucide:rotate-ccw" size="14" aria-hidden="true" />
        {{ t('configure.tuning_reset') }}
      </AppButton>
    </div>
    <p v-if="off" class="text-xs text-base-content/70">
      {{ t('configure.tuning_needs_source') }}
    </p>
    <div class="flex flex-wrap items-center gap-2">
      <span :id="`${uid}-preset`" class="text-xs text-base-content/70">{{ t('configure.tuning_preset') }}</span>
      <div class="join" role="radiogroup" :aria-labelledby="`${uid}-preset`" :aria-describedby="preset === null ? `${uid}-custom` : undefined">
        <label
          v-for="p in defaults.presets" :key="p.name"
          class="join-item btn btn-sm text-ui has-[:checked]:btn-primary has-[:disabled]:btn-disabled has-[:focus-visible]:outline-2 has-[:focus-visible]:outline-offset-2"
        >
          <input type="radio" class="sr-only" :name="`${uid}-preset`" :checked="preset === p.name" :disabled="off" @change="pick(p.name)">
          {{ presetLabel(p.name, t, te) }}
        </label>
      </div>
      <span v-if="preset === null" :id="`${uid}-custom`" class="badge badge-ghost badge-sm">{{ t('configure.preset_custom') }}</span>
    </div>
    <p v-if="presetHint" data-preset-hint class="text-xs text-base-content/70">
      {{ presetHint }}
    </p>
    <details class="collapse collapse-arrow rounded-box bg-base-200" :open="advanced" @toggle="advanced = ($event.target as HTMLDetailsElement).open">
      <summary class="collapse-title min-h-0 py-2 text-xs font-medium">
        {{ t('configure.tuning_advanced') }}
      </summary>
      <div class="collapse-content grid gap-3 sm:grid-cols-3">
        <div v-for="f in fields" :key="f" class="flex flex-col gap-1">
          <label class="flex flex-col gap-1">
            <span class="label text-ui text-base-content">{{ t(labels[f]) }}</span>
            <input
              type="number" class="input input-sm w-full tabular-nums" :class="{ 'input-error': errors[f] }"
              :value="texts[f]" :placeholder="String(defaults[f])" :step="steps[f]" :min="defaults.ranges[f][0]" :max="defaults.ranges[f][1]"
              :disabled="off" :aria-invalid="!!errors[f]" :aria-describedby="`${uid}-${f}-hint${errors[f] ? ` ${uid}-${f}-error` : ''}`"
              @input="edit(f, $event.target as HTMLInputElement)"
            >
          </label>
          <FieldError :id="`${uid}-${f}-error`" :message="errors[f] ? t('configure.tuning_range', { min: errors[f][0], max: errors[f][1] }) : ''" />
          <p :id="`${uid}-${f}-hint`" class="text-xs text-base-content/70">
            {{ t('configure.tuning_recommended', { n: recommended[f] }) }}
          </p>
        </div>
      </div>
    </details>
  </div>
</template>
