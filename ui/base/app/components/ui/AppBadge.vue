<script setup lang="ts">
import { computed } from 'vue';

const { label, icon = '', size = 'sm', outline = false } = defineProps<{
  label: string,
  icon?: string,
  size?: 'xs' | 'sm',
  outline?: boolean,
}>();

const sizes = { xs: 'badge-xs', sm: 'badge-sm' } as const;
const classes = computed(() => ['badge', sizes[size], { 'badge-outline': outline }]);
</script>

<template>
  <span :class="classes">
    <Icon v-if="icon" :name="icon" size="12" aria-hidden="true" />{{ label }}
  </span>
</template>
