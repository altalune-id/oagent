<script setup lang="ts">
import type { NavItem } from './types';

withDefaults(defineProps<{
  label?: string,
  items: NavItem[],
  version?: string,
}>(), {
  label: '',
  version: '',
});

const emit = defineEmits<{ select: [key: string] }>();
const slots = useSlots();
</script>

<template>
  <aside class="flex w-60 shrink-0 flex-col border-r border-base-300 bg-base-200/40">
    <div v-if="slots.back" class="px-4 pt-4">
      <slot name="back" />
    </div>
    <div v-if="label" class="truncate px-4 pb-2 text-xs font-semibold tracking-widest text-base-content/50 uppercase" :class="slots.back ? 'pt-2' : 'pt-5'">
      {{ label }}
    </div>
    <ul class="menu w-full gap-0.5 px-2">
      <li v-for="item in items" :key="item.key">
        <button type="button" :class="{ 'menu-active': item.active }" @click="emit('select', item.key)">
          <Icon v-if="item.icon" :name="item.icon" size="16" />
          {{ item.label }}
        </button>
      </li>
    </ul>
    <div v-if="slots.footer || version" data-sidebar-footer class="mt-auto px-4 py-3 text-xs text-base-content/40">
      <slot name="footer">
        <span v-if="version">oagent v{{ version }}</span>
      </slot>
    </div>
  </aside>
</template>
