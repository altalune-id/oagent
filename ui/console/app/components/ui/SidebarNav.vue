<script setup lang="ts">
import type { NavItem } from './types'

withDefaults(defineProps<{
  label?: string
  items: NavItem[]
}>(), {
  label: '',
})

const emit = defineEmits<{ select: [key: string] }>()
</script>

<template>
  <aside class="hidden w-60 shrink-0 flex-col border-r border-base-300 bg-base-200/40 md:flex">
    <div v-if="label" class="px-4 pt-5 pb-2 text-xs font-semibold tracking-widest text-base-content/50 uppercase">
      {{ label }}
    </div>
    <ul class="menu w-full gap-0.5 px-2">
      <li v-for="item in items" :key="item.key">
        <button type="button" :class="{ 'menu-active': item.active }" @click="emit('select', item.key)">
          {{ item.label }}
        </button>
      </li>
    </ul>
  </aside>
</template>
