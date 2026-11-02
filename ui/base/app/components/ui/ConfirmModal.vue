<script setup lang="ts">
import type { ConfirmHandle } from './types';
import AppButton from './AppButton.vue';
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
    <p v-if="body" class="text-ui">
      {{ body }}
    </p>
    <slot />
    <ErrorText :error="handle.error.value" />
    <template #footer>
      <AppButton variant="ghost" size="sm" :disabled="handle.busy.value" @click="handle.cancel()">
        {{ cancelLabel }}
      </AppButton>
      <AppButton variant="error" size="sm" :busy="handle.busy.value" @click="handle.confirm()">
        {{ confirmLabel }}
      </AppButton>
    </template>
  </AppModal>
</template>
