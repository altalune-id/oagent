<script setup lang="ts">
const { label = 'Saved' } = defineProps<{ label?: string }>();
</script>

<template>
  <span class="flex items-center gap-1 text-sm text-base-content/70" role="status">
    <Icon name="lucide:check" size="14" class="text-success" />
    {{ label }}
  </span>
</template>
