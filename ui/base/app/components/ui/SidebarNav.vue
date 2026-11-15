<script setup lang="ts">
import type { NavGroup } from './types';
import { useSlots } from 'vue';

const { label = '', groups, lead = [], dense = true, rail = false } = defineProps<{
  label?: string,
  groups: NavGroup[],
  lead?: NavGroup[],
  dense?: boolean,
  rail?: boolean,
}>();

const slots = useSlots();
const focusRing = 'focus-visible:outline-2 focus-visible:outline-solid! focus-visible:-outline-offset-2 focus-visible:outline-base-content';
</script>

<template>
  <aside class="flex h-full min-h-full w-60 shrink-0 flex-col border-e border-base-300 bg-base-200" :class="{ 'lg:w-16': rail }">
    <div v-if="slots.top" class="p-2">
      <slot name="top" />
    </div>
    <div v-if="slots.back" class="px-4 pt-4">
      <slot name="back" />
    </div>
    <nav :aria-label="label || undefined" class="min-h-0 flex-1 overflow-y-auto" :class="{ 'lg:overflow-visible': rail }">
      <template v-for="(list, l) in [lead, groups]" :key="l">
        <div v-if="l === 1 && slots.between" class="border-y border-base-300" :class="[dense ? 'my-1 p-2' : 'px-4 py-2', { 'lg:hidden': rail }]">
          <slot name="between" />
        </div>
        <ul v-if="list.length" class="menu menu-sm w-full text-ui" :class="{ 'py-0': dense }">
          <template v-for="(group, g) in list" :key="group.label ?? g">
            <li v-if="group.label" class="menu-title pt-3 text-xs text-base-content/70" :class="{ 'lg:sr-only': rail }">
              {{ group.label }}
            </li>
            <li v-for="item in group.items" :key="item.key" :class="{ 'lg:tooltip lg:tooltip-right': rail }" :data-tip="rail ? item.label : undefined">
              <NuxtLink
                :to="item.to" :class="[focusRing, item.children?.length ? { 'lg:menu-active': rail && item.active } : { 'menu-active': item.active }, { 'lg:justify-center': rail }]"
                :aria-current="item.active && !item.children?.length ? 'page' : undefined"
              >
                <Icon v-if="item.icon" :name="item.icon" size="16" class="shrink-0" aria-hidden="true" />
                <span class="flex-1" :class="{ 'lg:sr-only': rail }">{{ item.label }}</span>
                <span v-if="item.badge" class="badge badge-sm" :class="{ 'lg:hidden': rail }">{{ item.badge }}</span>
              </NuxtLink>
              <ul v-if="item.children?.length" :class="{ 'lg:hidden': rail }">
                <li v-for="child in item.children" :key="child.key">
                  <NuxtLink :to="child.to" :class="[focusRing, { 'menu-active': child.active }]" :aria-current="child.active ? 'page' : undefined">
                    <span class="flex-1">{{ child.label }}</span>
                    <span v-if="child.badge" class="badge badge-sm">{{ child.badge }}</span>
                  </NuxtLink>
                </li>
              </ul>
            </li>
          </template>
        </ul>
      </template>
    </nav>
    <div v-if="slots.footer" data-sidebar-footer class="flex flex-col gap-1 border-t border-base-300 p-2">
      <slot name="footer" />
    </div>
  </aside>
</template>
