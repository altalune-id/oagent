<script setup lang="ts">
import type { SelectAllState } from './selectAll';

const { state, label, context = '', disabled = false, size = 'sm' } = defineProps<{ state: SelectAllState, label: string, context?: string, disabled?: boolean, size?: 'xs' | 'sm' }>();
const emit = defineEmits<{ change: [on: boolean] }>();
</script>

<template>
  <label class="flex items-center gap-2 text-ui font-medium">
    <input
      type="checkbox" class="checkbox rounded-sm" :class="size === 'xs' ? 'checkbox-xs' : 'checkbox-sm'" :checked="state === 'all'" :indeterminate.prop="state === 'some'"
      :aria-checked="state === 'some' ? 'mixed' : undefined" :disabled="disabled"
      @change="emit('change', ($event.target as HTMLInputElement).checked)"
    >
    <span><span v-if="context" class="sr-only">{{ context }}</span><template v-if="context">{{ ' ' }}</template>{{ label }}</span>
  </label>
</template>
