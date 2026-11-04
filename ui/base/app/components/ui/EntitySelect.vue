<script setup lang="ts">
import type { EntityItem } from './types';
import { computed, ref } from 'vue';
import AppDropdown from './AppDropdown.vue';
import SearchInput from './SearchInput.vue';

const { label, items, searchLabel = '', title = '', empty = '', triggerClass = '', open = false } = defineProps<{
  label: string,
  items: EntityItem[],
  searchLabel?: string,
  title?: string,
  empty?: string,
  triggerClass?: string,
  open?: boolean,
}>();

const emit = defineEmits<{ select: [key: string] }>();

const query = ref('');
const shown = computed(() => {
  const q = query.value.trim().toLowerCase();
  return q ? items.filter(item => item.label.toLowerCase().includes(q)) : items;
});
</script>

<template>
  <AppDropdown class="w-full" bare :label="label" :open="open" content-class="card card-sm z-1 w-64" :trigger-class="['w-full justify-between text-ui', triggerClass].join(' ')">
    <template #trigger>
      <slot name="trigger" />
      <span class="inline-flex shrink-0 items-center gap-1">
        <slot name="end" />
        <Icon name="lucide:chevrons-up-down" size="14" class="shrink-0" aria-hidden="true" />
      </span>
    </template>
    <div class="card-body gap-2 p-2">
      <SearchInput v-if="searchLabel" v-model="query" :label="searchLabel" class="w-full" />
      <ul class="menu menu-sm w-full p-0 text-ui">
        <li v-if="title" class="menu-title text-base-content/70">
          {{ title }}
        </li>
        <li v-for="item in shown" :key="item.key">
          <button type="button" :class="{ 'menu-active': item.active }" :aria-current="item.active ? 'true' : undefined" @click="emit('select', item.key)">
            <span class="min-w-0 truncate" :class="{ 'font-mono': item.mono }">{{ item.label }}</span>
            <span v-if="item.badge" class="badge badge-xs">{{ item.badge }}</span>
          </button>
        </li>
        <li v-if="!shown.length && empty" class="menu-disabled">
          <span>{{ empty }}</span>
        </li>
        <slot name="actions" />
      </ul>
    </div>
  </AppDropdown>
</template>
