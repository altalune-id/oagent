<script setup lang="ts">
import type { LanguageOption } from './types';
import { computed } from 'vue';
import AppDropdown from './AppDropdown.vue';

const props = withDefaults(defineProps<{
  options: LanguageOption[],
  modelValue: string,
  label?: string,
  open?: boolean,
}>(), {
  label: 'Language',
  open: false,
});

const emit = defineEmits<{ select: [code: string] }>();

const current = computed(() => props.options.find(option => option.code === props.modelValue));
</script>

<template>
  <AppDropdown class="dropdown-end" :label="label" :open="open" trigger-class="gap-2 max-sm:px-2">
    <template #trigger>
      <Icon v-if="current?.icon" :name="current.icon" size="18" class="rounded-full" />
      <Icon v-else name="lucide:languages" size="16" />
      <Icon name="lucide:chevron-down" size="14" class="max-sm:hidden" />
    </template>
    <ul tabindex="0" class="dropdown-content menu z-50 mt-1 w-48 rounded-box border border-base-300 bg-base-100 p-2 shadow">
      <li v-for="option in options" :key="option.code">
        <button type="button" :class="{ 'menu-active': option.code === modelValue }" @click="emit('select', option.code)">
          <Icon v-if="option.icon" :name="option.icon" size="18" class="rounded-full" />
          {{ option.name }}
        </button>
      </li>
    </ul>
  </AppDropdown>
</template>
