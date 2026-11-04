<script setup lang="ts">
const { mode, label = 'Switch between light and dark' } = defineProps<{
  mode: 'light' | 'dark',
  label?: string,
}>();

const emit = defineEmits<{ toggle: [] }>();
</script>

<template>
  <button type="button" class="btn btn-xs btn-ghost btn-square" :aria-label="label" :title="label" :aria-pressed="mode === 'dark'" @click="emit('toggle')">
    <Icon name="lucide:sun-moon" size="14" class="shrink-0" aria-hidden="true" />
  </button>
</template>
