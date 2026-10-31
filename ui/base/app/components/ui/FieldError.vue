<script setup lang="ts">
defineProps<{ id: string, message?: string }>();
</script>

<template>
  <span v-if="message" :id="id" class="mt-1 block text-xs text-error">{{ message }}</span>
</template>
