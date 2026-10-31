<script setup lang="ts">
import type { LanguageOption } from './types';
import { computed } from 'vue';

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
  <div class="dropdown dropdown-end" :class="{ 'dropdown-open': open }">
    <div tabindex="0" role="button" class="btn btn-ghost btn-sm gap-2 max-sm:px-2" :aria-label="label">
      <Icon v-if="current?.icon" :name="current.icon" size="18" class="rounded-full" />
      <Icon v-else name="lucide:languages" size="16" />
      <Icon name="lucide:chevron-down" size="14" class="max-sm:hidden" />
    </div>
    <ul tabindex="0" class="dropdown-content menu z-50 mt-1 w-48 rounded-box border border-base-300 bg-base-100 p-2 shadow">
      <li v-for="option in options" :key="option.code">
        <button type="button" :class="{ 'menu-active': option.code === modelValue }" @click="emit('select', option.code)">
          <Icon v-if="option.icon" :name="option.icon" size="18" class="rounded-full" />
          {{ option.name }}
        </button>
      </li>
    </ul>
  </div>
</template>
