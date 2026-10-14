<script setup lang="ts">
import { computed } from 'vue';

type Variant = 'primary' | 'neutral' | 'ghost' | 'outline';
type Size = 'sm' | 'md' | 'lg';

const props = withDefaults(defineProps<{
  variant?: Variant,
  size?: Size,
  disabled?: boolean,
  type?: 'button' | 'submit' | 'reset',
}>(), {
  variant: 'primary',
  size: 'md',
  disabled: false,
  type: 'button',
});

const classes = computed(() => [
  'btn',
  `btn-${props.variant}`,
  props.size === 'md' ? '' : `btn-${props.size}`,
]);
</script>

<template>
  <button :type="type" :class="classes" :disabled="disabled">
    <slot />
  </button>
</template>
