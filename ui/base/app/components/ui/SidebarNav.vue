<script setup lang="ts">
import type { NavGroup } from './types';
import { useSlots } from 'vue';

const { label = '', groups } = defineProps<{
  label?: string,
  groups: NavGroup[],
}>();

const slots = useSlots();
</script>

<template>
  <aside class="flex h-full min-h-full w-60 shrink-0 flex-col border-e border-base-300 bg-base-200">
    <div v-if="slots.top" class="p-2">
      <slot name="top" />
    </div>
    <div v-if="slots.back" class="px-4 pt-4">
      <slot name="back" />
    </div>
    <nav :aria-label="label || undefined" class="min-h-0 flex-1 overflow-y-auto">
      <ul class="menu menu-sm w-full py-0 text-ui">
        <template v-for="(group, g) in groups" :key="group.label ?? g">
          <li v-if="group.label" class="menu-title pt-3 text-xs text-base-content/70">
            {{ group.label }}
          </li>
          <li v-for="item in group.items" :key="item.key">
            <NuxtLink :to="item.to" :class="{ 'menu-active': item.active }" :aria-current="item.active ? 'page' : undefined">
              <Icon v-if="item.icon" :name="item.icon" size="16" class="shrink-0" />
              <span class="flex-1">{{ item.label }}</span>
              <span v-if="item.badge" class="badge badge-sm">{{ item.badge }}</span>
            </NuxtLink>
          </li>
        </template>
      </ul>
    </nav>
    <div v-if="slots.footer" data-sidebar-footer class="flex flex-col gap-1 border-t border-base-300 p-2">
      <slot name="footer" />
    </div>
  </aside>
</template>
