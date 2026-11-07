<script setup lang="ts">
import type { Crumb, TopBarPlace } from './types';
import AppButton from './AppButton.vue';
import Breadcrumbs from './Breadcrumbs.vue';

const { crumbs, place, crumbsLabel = 'Breadcrumb', menuLabel = 'Toggle navigation', searchLabel = 'Search', jumpLabel = 'Search and jump', shortcutLabel = 'Command K', menu = true, menuOpen = false, sidebarId = 'app-sidebar', rail = false } = defineProps<{
  crumbs: Crumb[],
  place?: TopBarPlace,
  crumbsLabel?: string,
  menuLabel?: string,
  searchLabel?: string,
  jumpLabel?: string,
  shortcutLabel?: string,
  menu?: boolean,
  menuOpen?: boolean,
  sidebarId?: string,
  rail?: boolean,
}>();

const emit = defineEmits<{ menu: [], search: [] }>();
</script>

<template>
  <header class="navbar min-h-12 gap-2 border-b border-base-300 bg-base-100 px-3 md:px-4">
    <AppButton v-if="menu" variant="ghost" size="sm" square :class="{ 'lg:hidden': !rail }" :aria-label="menuLabel" :aria-controls="sidebarId" :aria-expanded="menuOpen" @click="emit('menu')">
      <Icon :name="rail ? 'lucide:panel-left' : 'lucide:menu'" size="16" class="shrink-0" aria-hidden="true" />
    </AppButton>
    <Breadcrumbs class="flex-1 max-md:hidden" :crumbs="crumbs" :label="crumbsLabel" />
    <div class="min-w-0 flex-1 text-ui md:hidden">
      <div v-if="place" class="truncate font-medium">
        {{ place.label }}
      </div>
    </div>
    <span v-if="place?.badge" class="badge badge-sm md:hidden">{{ place.badge }}</span>
    <AppButton variant="ghost" size="sm" class="hidden text-base-content/70 md:inline-flex" :title="`${jumpLabel}, ${shortcutLabel}`" aria-keyshortcuts="Meta+K Control+K" @click="emit('search')">
      <Icon name="lucide:search" size="16" class="shrink-0" aria-hidden="true" />
      <span>{{ searchLabel }}</span>
      <kbd class="kbd kbd-xs" aria-hidden="true">⌘K</kbd>
    </AppButton>
    <div v-if="$slots.controls" class="hidden md:block">
      <div class="flex items-center gap-1">
        <slot name="controls" />
      </div>
    </div>
  </header>
</template>
