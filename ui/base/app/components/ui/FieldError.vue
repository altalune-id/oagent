<script setup lang="ts">
defineProps<{ id: string, message?: string }>();
</script>

<template>
  <span v-if="message" :id="id" class="mt-1 flex items-start gap-1 text-xs text-base-content">
    <Icon name="lucide:circle-alert" size="14" class="shrink-0 text-error" />
    {{ message }}
  </span>
</template>
