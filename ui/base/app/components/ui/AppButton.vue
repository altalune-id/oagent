<script setup lang="ts">
import { computed } from 'vue';

type Variant = 'primary' | 'neutral' | 'ghost' | 'outline' | 'error' | 'plain';
type Size = 'xs' | 'sm' | 'md' | 'lg';

const props = withDefaults(defineProps<{
  variant?: Variant,
  size?: Size,
  soft?: boolean,
  square?: boolean,
  disabled?: boolean,
  busy?: boolean,
  type?: 'button' | 'submit' | 'reset',
  to?: string,
  href?: string,
}>(), {
  variant: 'primary',
  size: 'md',
  soft: false,
  square: false,
  disabled: false,
  busy: false,
  type: 'button',
  to: '',
  href: '',
});

const classes = computed(() => [
  'btn',
  props.variant === 'plain' ? '' : `btn-${props.variant}`,
  props.size === 'md' ? '' : `btn-${props.size}`,
  { 'btn-soft': props.soft, 'btn-square': props.square },
]);
</script>

<template>
  <NuxtLink v-if="to" :to="to" :class="classes">
    <slot />
  </NuxtLink>
  <a v-else-if="href" :href="href" :class="classes">
    <slot />
  </a>
  <button v-else :type="type" :class="classes" :disabled="disabled || busy">
    <span v-if="busy" class="loading loading-spinner loading-xs" aria-hidden="true" />
    <slot />
  </button>
</template>
