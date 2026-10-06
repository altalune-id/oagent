<script setup lang="ts">
import { computed } from 'vue'
import type { NavItem } from '~/components/primitives/types'

const route = useRoute()
const org = computed(() => route.params.org as string)

const targets: Record<string, string> = {
  overview: '',
  projects: '/projects',
  settings: '/settings',
}

const items = computed<NavItem[]>(() => [
  { key: 'overview', label: 'Overview', active: route.path === `/o/${org.value}` },
  { key: 'projects', label: 'Projects', active: route.path.startsWith(`/o/${org.value}/projects`) },
  { key: 'settings', label: 'Settings', active: route.path.startsWith(`/o/${org.value}/settings`) },
])

function onSelect(key: string) {
  return navigateTo(`/o/${org.value}${targets[key] ?? ''}`)
}
</script>

<template>
  <SidebarNav :label="org" :items="items" @select="onSelect" />
</template>
