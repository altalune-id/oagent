<script setup lang="ts">
import type { ConfirmHandle } from './types';
import AppModal from './AppModal.vue';
import ErrorText from './ErrorText.vue';

const { handle, title, body = '', confirmLabel, cancelLabel = 'Cancel' } = defineProps<{
  handle: ConfirmHandle,
  title: string,
  body?: string,
  confirmLabel: string,
  cancelLabel?: string,
}>();
</script>

<template>
  <AppModal :open="handle.target.value !== null" :title="title" @close="handle.cancel()">
    <p v-if="body" class="text-sm">
      {{ body }}
    </p>
    <slot />
    <ErrorText :error="handle.error.value" />
    <template #footer>
      <button type="button" class="btn btn-ghost btn-sm" :disabled="handle.busy.value" @click="handle.cancel()">
        {{ cancelLabel }}
      </button>
      <button type="button" class="btn btn-error btn-sm" :disabled="handle.busy.value" @click="handle.confirm()">
        <span v-if="handle.busy.value" class="loading loading-spinner loading-xs" aria-hidden="true" />
        {{ confirmLabel }}
      </button>
    </template>
  </AppModal>
</template>
