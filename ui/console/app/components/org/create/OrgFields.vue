<script setup lang="ts">
import FieldError from '#base/components/ui/FieldError.vue';

const { fieldErrors, busy = false } = defineProps<{ fieldErrors: Record<string, string>, busy?: boolean }>();
const name = defineModel<string>('name', { required: true });
const slug = defineModel<string>('slug', { required: true });
const { t } = useI18n();
const ids = { name: useId(), nameLegend: useId(), nameErr: useId(), slug: useId(), slugLegend: useId(), slugErr: useId(), slugHelp: useId() };
</script>

<template>
  <fieldset class="fieldset">
    <legend :id="ids.nameLegend" class="fieldset-legend">
      {{ t('common.name') }}
    </legend>
    <input
      :id="ids.name" v-model="name" type="text" class="input w-full" :class="{ 'input-error': fieldErrors.name }"
      :placeholder="t('org.name_placeholder')" :disabled="busy" autocomplete="organization"
      :aria-labelledby="ids.nameLegend" :aria-invalid="!!fieldErrors.name" :aria-describedby="ids.nameErr"
    >
    <FieldError :id="ids.nameErr" :message="fieldErrors.name" />
  </fieldset>
  <fieldset class="fieldset">
    <legend :id="ids.slugLegend" class="fieldset-legend">
      {{ t('common.slug') }}
    </legend>
    <input
      :id="ids.slug" v-model="slug" type="text" class="input w-full font-mono" :class="{ 'input-error': fieldErrors.slug }"
      :placeholder="t('org.slug_placeholder')" :disabled="busy"
      :aria-labelledby="ids.slugLegend" :aria-invalid="!!fieldErrors.slug" :aria-describedby="`${ids.slugHelp} ${ids.slugErr}`"
    >
    <FieldError :id="ids.slugErr" :message="fieldErrors.slug" />
    <p :id="ids.slugHelp" class="label whitespace-normal">
      {{ t('org.slug_random_help') }}
    </p>
  </fieldset>
</template>
