<script setup lang="ts">
import SidebarNav from '~/components/ui/SidebarNav.vue'
import type { NavItem } from '~/components/ui/types'
import { useOrgsStore } from '~/stores/orgs'

const route = useRoute()
const { t } = useI18n()
const { appVersion } = useRuntimeConfig().public
const orgs = useOrgsStore()
const org = computed(() => route.params.org as string)
const label = computed(() => orgs.bySlug(org.value)?.name ?? '')

const targets: Record<string, string> = {
  overview: '',
  projects: '/projects',
  settings: '/settings',
}

const items = computed<NavItem[]>(() => [
  { key: 'overview', label: t('nav.overview'), icon: 'lucide:layout-dashboard', active: route.path === `/o/${org.value}` },
  { key: 'projects', label: t('nav.projects'), icon: 'lucide:folder', active: route.path.startsWith(`/o/${org.value}/projects`) },
  { key: 'settings', label: t('nav.settings'), icon: 'lucide:settings', active: route.path.startsWith(`/o/${org.value}/settings`) },
])

function onSelect(key: string) {
  return navigateTo(`/o/${org.value}${targets[key] ?? ''}`)
}
</script>

<template>
  <SidebarNav :label="label" :items="items" @select="onSelect">
    <template #back>
      <NuxtLink to="/orgs" class="inline-flex items-center gap-1 text-xs text-base-content/60 transition-colors hover:text-base-content">
        <Icon name="lucide:chevron-left" size="14" />
        {{ t('org.all') }}
      </NuxtLink>
    </template>
    <template v-if="appVersion" #footer>
      oagent v{{ appVersion }}
    </template>
  </SidebarNav>
</template>
