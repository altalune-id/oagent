<script setup lang="ts">
defineProps<{ id: string, message?: string }>();
</script>

<template>
  <span v-if="message" :id="id" class="mt-2 flex items-start gap-1 text-xs leading-[normal] text-error">
    <Icon name="lucide:circle-alert" size="14" class="shrink-0" />
    <slot>{{ message }}</slot>
  </span>
</template>
