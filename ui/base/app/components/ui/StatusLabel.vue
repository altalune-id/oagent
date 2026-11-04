<script setup lang="ts">
import type { StatusTone } from './types';
import { computed } from 'vue';

const { tone, label, detail = '' } = defineProps<{ tone: StatusTone, label: string, detail?: string }>();

const tones: Record<StatusTone, string> = {
  success: 'status-success',
  warning: 'status-warning',
  error: 'status-error',
  info: 'status-info',
  neutral: 'status-neutral',
};
const dot = computed(() => ['status', tones[tone]]);
</script>

<template>
  <span class="inline-flex min-w-0 flex-wrap items-center gap-x-2 text-ui">
    <span aria-hidden="true" :class="dot" />
    <span class="font-medium">{{ label }}</span>
    <span v-if="detail || $slots.default" class="text-base-content/70"><slot>{{ detail }}</slot></span>
  </span>
</template>
