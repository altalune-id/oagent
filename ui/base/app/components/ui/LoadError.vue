<script setup lang="ts">
import type { ShownError } from '../../lib/apperror/message';
import ErrorText from './ErrorText.vue';

const { error, busy = false, retryLabel = 'Retry' } = defineProps<{
  error: ShownError | null,
  busy?: boolean,
  retryLabel?: string,
}>();

const emit = defineEmits<{ retry: [] }>();
</script>

<template>
  <div v-if="error" class="flex flex-wrap items-center justify-between gap-3 rounded-box border border-base-300 bg-base-100 p-3 text-ui">
    <ErrorText :error="error" />
    <button type="button" class="btn btn-sm text-ui" :disabled="busy" @click="emit('retry')">
      <span v-if="busy" class="loading loading-spinner loading-xs" aria-hidden="true" />
      <Icon v-else name="lucide:rotate-ccw" size="14" />
      {{ retryLabel }}
    </button>
  </div>
</template>
