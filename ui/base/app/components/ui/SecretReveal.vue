<script setup lang="ts">
import type { SecretRevealLabels } from './types';
import { computed } from 'vue';
import CopyButton from './CopyButton.vue';
import SecretField from './SecretField.vue';

const { value, labels, env = {} } = defineProps<{
  value: string,
  labels: SecretRevealLabels,
  env?: Record<string, string>,
}>();

const confirmed = defineModel<boolean>('confirmed', { default: false });

const dotenv = computed(() => Object.entries(env).map(([k, v]) => `${k}=${v}`).join('\n'));
</script>

<template>
  <div class="flex flex-col gap-3">
    <div v-if="!value" role="alert" class="alert alert-error alert-soft items-start">
      <Icon name="lucide:circle-alert" size="18" class="mt-0.5 shrink-0" />
      <div class="flex flex-col gap-2">
        <span>{{ labels.missing }}</span>
        <slot name="retry" />
      </div>
    </div>
    <template v-else>
      <div role="note" class="alert alert-warning alert-soft items-start">
        <Icon name="lucide:triangle-alert" size="18" class="mt-0.5 shrink-0" />
        <span>{{ labels.once }}</span>
      </div>
      <SecretField :value="value" :label="labels.copy" />
      <div v-if="dotenv" class="flex items-center gap-2 text-ui">
        <CopyButton :value="dotenv" :label="labels.copyEnv" />
        <span aria-hidden="true">{{ labels.copyEnv }}</span>
      </div>
      <label class="label cursor-pointer gap-2 text-sm text-base-content">
        <input v-model="confirmed" type="checkbox" class="checkbox checkbox-sm rounded-sm">
        {{ labels.confirm }}
      </label>
    </template>
  </div>
</template>
