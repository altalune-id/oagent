<script setup lang="ts">
import type { EntityItem } from './types';

withDefaults(defineProps<{
  label: string,
  items: EntityItem[],
  icon?: string,
  placeholder?: boolean,
  empty?: string,
  pinLabel?: string,
  unpinLabel?: string,
  open?: boolean,
}>(), {
  icon: '',
  placeholder: false,
  empty: '',
  pinLabel: 'Pin',
  unpinLabel: 'Unpin',
  open: false,
});

const emit = defineEmits<{
  select: [key: string],
  pin: [key: string],
}>();
</script>

<template>
  <div class="dropdown min-w-0" :class="{ 'dropdown-open': open }">
    <div tabindex="0" role="button" class="btn btn-ghost btn-sm max-w-full min-w-0 flex-nowrap gap-1 sm:gap-2" :aria-label="label">
      <Icon v-if="icon" :name="icon" size="16" class="shrink-0 max-sm:hidden" />
      <span class="min-w-0 truncate sm:max-w-32" :class="{ 'text-base-content/50': placeholder }">{{ label }}</span>
      <Icon name="lucide:chevron-down" size="14" class="shrink-0" />
    </div>
    <ul tabindex="0" class="dropdown-content menu z-50 mt-1 w-60 rounded-box max-sm:fixed max-sm:inset-x-2 max-sm:top-full max-sm:w-auto border border-base-300 bg-base-100 p-2 shadow">
      <li v-for="item in items" :key="item.key">
        <div class="flex items-center gap-1 rounded-box" :class="{ 'menu-active': item.active }">
          <button type="button" class="min-w-0 flex-1 truncate text-left" @click="emit('select', item.key)">
            {{ item.label }}
          </button>
          <button
            type="button"
            class="shrink-0 rounded p-0.5 hover:bg-base-200"
            :aria-label="item.pinned ? unpinLabel : pinLabel"
            :title="item.pinned ? unpinLabel : pinLabel"
            @click="emit('pin', item.key)"
          >
            <Icon name="lucide:star" size="14" :class="item.pinned ? 'text-amber-400 [&_path]:fill-amber-400' : 'text-base-content/50'" />
          </button>
        </div>
      </li>
      <li v-if="!items.length && empty" class="menu-disabled">
        <span>{{ empty }}</span>
      </li>
      <slot name="actions" />
    </ul>
  </div>
</template>
