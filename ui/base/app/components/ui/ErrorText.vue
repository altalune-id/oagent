<script setup lang="ts">
import type { ShownError } from '../../lib/apperror/message';

defineProps<{ error: ShownError | null }>();
</script>

<template>
  <p v-if="error" class="text-sm text-error">
    {{ error.message }}
    <span v-if="error.reference" class="block text-xs text-base-content/60">{{ error.reference }}</span>
  </p>
</template>
