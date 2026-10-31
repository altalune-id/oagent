<script setup lang="ts">
import type { SelectOption } from './types';
import { computed } from 'vue';

const props = withDefaults(defineProps<{
  modelValue?: string,
  options: SelectOption[],
  placeholder?: string,
  ariaLabel?: string,
  disabled?: boolean,
  size?: 'sm' | 'md' | 'lg',
}>(), {
  modelValue: '',
  placeholder: '',
  ariaLabel: '',
  disabled: false,
  size: 'sm',
});

const emit = defineEmits<{
  'update:modelValue': [value: string],
  'change': [value: string],
}>();

const classes = computed(() => [
  'select',
  props.size === 'md' ? '' : `select-${props.size}`,
  'w-44',
  'border-base-300',
]);

function onChange(event: Event) {
  const value = (event.target as HTMLSelectElement).value;
  emit('update:modelValue', value);
  emit('change', value);
}
</script>

<template>
  <select
    :class="classes"
    :value="modelValue"
    :aria-label="ariaLabel"
    :disabled="disabled"
    @change="onChange"
  >
    <option value="" disabled>
      {{ placeholder }}
    </option>
    <option v-for="option in options" :key="option.value" :value="option.value">
      {{ option.label }}
    </option>
  </select>
</template>
