<script setup lang="ts">
import CopyButton from './CopyButton.vue';

const { value, label = 'Copy' } = defineProps<{ value: string, label?: string }>();
</script>

<template>
  <div class="flex items-center gap-2 rounded-box border border-base-300 bg-base-200 py-1.5 pr-1.5 pl-3">
    <code class="min-w-0 flex-1 font-mono text-xs leading-relaxed break-all">{{ value }}</code>
    <CopyButton :value="value" :label="label" class="shrink-0" />
  </div>
</template>
