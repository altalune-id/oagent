<script setup lang="ts">
import type { ThemeFamilyOption } from './types';
import { computed } from 'vue';
import AppDropdown from './AppDropdown.vue';

const props = withDefaults(defineProps<{
  families: ThemeFamilyOption[],
  modelValue: string,
  label?: string,
  open?: boolean,
}>(), {
  label: 'Theme',
  open: false,
});

const emit = defineEmits<{ select: [id: string] }>();

const current = computed(() => props.families.find(family => family.id === props.modelValue));
</script>

<template>
  <AppDropdown class="dropdown-end" :label="label" :open="open" trigger-class="gap-2 max-sm:px-2">
    <template #trigger>
      <Icon name="lucide:palette" size="16" />
      <span v-if="current" class="hidden sm:inline">{{ current.label }}</span>
      <Icon name="lucide:chevron-down" size="14" class="max-sm:hidden" />
    </template>
    <ul tabindex="0" class="dropdown-content menu z-50 mt-1 w-56 rounded-box border border-base-300 bg-base-100 p-2 shadow">
      <li v-for="family in families" :key="family.id">
        <button type="button" :class="{ 'menu-active': family.id === modelValue }" @click="emit('select', family.id)">
          <span :data-theme="family.theme" class="flex items-center gap-0.5" aria-hidden="true">
            <span class="size-3 rounded-full bg-primary ring-1 ring-base-content/10" />
            <span class="size-3 rounded-full bg-secondary ring-1 ring-base-content/10" />
            <span class="size-3 rounded-full bg-accent ring-1 ring-base-content/10" />
          </span>
          {{ family.label }}
        </button>
      </li>
    </ul>
  </AppDropdown>
</template>
