<script setup lang="ts">
import type { ThemeFamilyOption } from './types';

const { families, modelValue, label = 'Theme family' } = defineProps<{
  families: ThemeFamilyOption[],
  modelValue: string,
  label?: string,
}>();

const emit = defineEmits<{ select: [id: string] }>();
</script>

<template>
  <label class="inline-flex items-center">
    <span class="sr-only">{{ label }}</span>
    <select class="select select-xs select-ghost w-28 text-xs" :value="modelValue" @change="emit('select', ($event.target as HTMLSelectElement).value)">
      <option v-for="family in families" :key="family.id" :value="family.id">
        {{ family.label }}
      </option>
    </select>
  </label>
</template>
