<script setup lang="ts">
import type { ShownError } from '../../lib/apperror/message';

defineProps<{ error: ShownError | null }>();
</script>

<template>
  <p v-if="error" role="alert" class="flex items-start gap-1.5 text-sm text-base-content">
    <Icon name="lucide:circle-alert" size="16" class="mt-0.5 shrink-0 text-error" />
    <span>
      {{ error.message }}
      <span v-if="error.reference" class="block text-xs text-base-content/60">{{ error.reference }}</span>
    </span>
  </p>
</template>
