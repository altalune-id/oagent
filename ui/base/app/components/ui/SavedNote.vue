<script setup lang="ts">
const { label = 'Saved' } = defineProps<{ label?: string }>();
</script>

<template>
  <span class="flex items-center gap-1 text-ui text-base-content/70" role="status">
    <Icon name="lucide:check" size="14" class="shrink-0 text-success" aria-hidden="true" />
    {{ label }}
  </span>
</template>
