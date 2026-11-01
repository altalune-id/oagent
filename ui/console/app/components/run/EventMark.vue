<script setup lang="ts">
import type { Lane } from './runView';

const { lane } = defineProps<{ lane: Lane | null }>();
</script>

<template>
  <Icon v-if="lane === 'error'" name="lucide:x" size="12" class="shrink-0 text-error" aria-hidden="true" />
  <span
    v-else
    aria-hidden="true"
    class="inline-block size-2.5 shrink-0"
    :class="{
      'rounded-full bg-info': lane === 'model',
      'rounded-[2px] bg-base-content/70': lane === 'tool',
      'rotate-45 rounded-[1px] bg-warning': lane === 'approval',
      'rounded-full border border-base-content/70': lane === null,
    }"
  />
</template>
