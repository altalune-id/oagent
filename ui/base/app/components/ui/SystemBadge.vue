<script setup lang="ts">
const { label = 'System' } = defineProps<{ label?: string }>();
</script>

<template>
  <span class="badge badge-sm badge-ghost gap-1">
    <Icon name="lucide:shield" size="12" />
    {{ label }}
  </span>
</template>
