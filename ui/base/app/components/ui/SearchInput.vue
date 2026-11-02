<script setup lang="ts">
const { label, placeholder = '' } = defineProps<{ label: string, placeholder?: string }>();
const query = defineModel<string>({ default: '' });
</script>

<template>
  <label class="input input-sm text-ui w-full sm:w-64">
    <Icon name="lucide:search" size="14" class="shrink-0" aria-hidden="true" />
    <span class="sr-only">{{ label }}</span>
    <input v-model="query" type="search" :placeholder="placeholder || label" autocomplete="off">
  </label>
</template>
