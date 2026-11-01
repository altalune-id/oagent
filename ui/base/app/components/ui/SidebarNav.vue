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
  <aside class="flex min-h-full w-64 shrink-0 flex-col border-r border-base-300 bg-base-200">
    <div v-if="slots.top" class="px-3 pt-3">
      <slot name="top" />
    </div>
    <div v-if="slots.back" class="px-4 pt-4">
      <slot name="back" />
    </div>
    <nav :aria-label="label || undefined" class="pt-2">
      <ul class="menu w-full gap-0.5 px-2">
        <template v-for="(group, g) in groups" :key="group.label ?? g">
          <li v-if="group.label" class="menu-title text-base-content/70" :class="{ 'mt-2': g > 0 }">
            {{ group.label }}
          </li>
          <li v-for="item in group.items" :key="item.key">
            <NuxtLink :to="item.to" :class="{ 'menu-active': item.active }" :aria-current="item.active ? 'page' : undefined">
              <Icon v-if="item.icon" :name="item.icon" size="16" />
              <span class="flex-1">{{ item.label }}</span>
              <span v-if="item.badge" class="badge badge-sm badge-neutral">{{ item.badge }}</span>
            </NuxtLink>
          </li>
        </template>
      </ul>
    </nav>
    <div v-if="slots.footer" data-sidebar-footer class="mt-auto px-4 py-3 text-xs text-base-content/70">
      <slot name="footer" />
    </div>
  </aside>
</template>
