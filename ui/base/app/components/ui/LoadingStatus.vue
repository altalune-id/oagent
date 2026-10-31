<script setup lang="ts">
const { label } = defineProps<{ label: string }>();
</script>

<template>
  <span role="status" class="inline-flex items-center">
    <span class="loading loading-spinner loading-sm" aria-hidden="true" />
    <span class="sr-only">{{ label }}</span>
  </span>
</template>
