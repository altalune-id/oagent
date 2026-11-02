<script setup lang="ts">
import type { Crumb } from './types';
import AppButton from './AppButton.vue';
import Breadcrumbs from './Breadcrumbs.vue';

const { crumbs, crumbsLabel = 'Breadcrumb', menuLabel = 'Toggle navigation', searchLabel = 'Search', jumpLabel = 'Search and jump', shortcutLabel = 'Command K', menu = true } = defineProps<{
  crumbs: Crumb[],
  crumbsLabel?: string,
  menuLabel?: string,
  searchLabel?: string,
  jumpLabel?: string,
  shortcutLabel?: string,
  menu?: boolean,
}>();

const emit = defineEmits<{ menu: [], search: [] }>();
</script>

<template>
  <div class="navbar min-h-12 gap-2 border-b border-base-300 bg-base-100 px-4">
    <AppButton v-if="menu" variant="ghost" size="sm" square class="lg:hidden" :aria-label="menuLabel" @click="emit('menu')">
      <Icon name="lucide:menu" size="16" class="shrink-0" aria-hidden="true" />
    </AppButton>
    <Breadcrumbs class="flex-1" :crumbs="crumbs" :label="crumbsLabel" />
    <AppButton variant="ghost" size="sm" square class="md:hidden" :aria-label="jumpLabel" @click="emit('search')">
      <Icon name="lucide:search" size="16" class="shrink-0" aria-hidden="true" />
    </AppButton>
    <AppButton variant="ghost" size="sm" class="hidden text-base-content/70 md:inline-flex" :aria-label="`${jumpLabel}, ${shortcutLabel}`" @click="emit('search')">
      <Icon name="lucide:search" size="16" class="shrink-0" aria-hidden="true" />
      <span>{{ searchLabel }}</span>
      <kbd class="kbd kbd-xs">⌘K</kbd>
    </AppButton>
    <div v-if="$slots.controls" class="hidden md:block">
      <div class="flex items-center gap-1">
        <slot name="controls" />
      </div>
    </div>
  </div>
</template>
