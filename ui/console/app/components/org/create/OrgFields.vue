<script setup lang="ts">
import FieldError from '#base/components/ui/FieldError.vue';

const { fieldErrors, busy = false } = defineProps<{ fieldErrors: Record<string, string>, busy?: boolean }>();
const name = defineModel<string>('name', { required: true });
const slug = defineModel<string>('slug', { required: true });
const { t } = useI18n();
const ids = { name: useId(), nameErr: useId(), slug: useId(), slugErr: useId(), slugHelp: useId() };
</script>

<template>
  <div class="fieldset">
    <label :for="ids.name" class="label text-ui text-base-content">{{ t('common.name') }}</label>
    <input
      :id="ids.name" v-model="name" type="text" class="input input-sm w-full text-ui" :class="{ 'input-error': fieldErrors.name }"
      :placeholder="t('org.name_placeholder')" :disabled="busy" autocomplete="organization"
      :aria-invalid="!!fieldErrors.name" :aria-describedby="ids.nameErr"
    >
    <FieldError :id="ids.nameErr" :message="fieldErrors.name" />
  </div>
  <div class="fieldset">
    <label :for="ids.slug" class="label text-ui text-base-content">{{ t('common.slug') }}</label>
    <input
      :id="ids.slug" v-model="slug" type="text" class="input input-sm w-full font-mono text-ui" :class="{ 'input-error': fieldErrors.slug }"
      :placeholder="t('org.slug_placeholder')" :disabled="busy"
      :aria-invalid="!!fieldErrors.slug" :aria-describedby="`${ids.slugHelp} ${ids.slugErr}`"
    >
    <FieldError :id="ids.slugErr" :message="fieldErrors.slug" />
    <p :id="ids.slugHelp" class="label whitespace-normal text-xs text-base-content/70">
      {{ t('org.slug_random_help') }}
    </p>
  </div>
</template>
