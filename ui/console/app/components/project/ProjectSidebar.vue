<script setup lang="ts">
import SidebarNav from '~/components/ui/SidebarNav.vue'
import type { NavItem } from '~/components/ui/types'
import { useOrgsStore } from '~/stores/orgs'
import { useProjectsStore } from '~/stores/projects'

const route = useRoute()
const { t } = useI18n()
const { appVersion } = useRuntimeConfig().public
const orgs = useOrgsStore()
const projects = useProjectsStore()

const org = computed(() => orgs.bySlug(route.params.org as string))
const projectSlug = computed(() => route.params.project as string)
const project = computed(() => projects.items.find(p => p.orgId === org.value?.id && p.slug === projectSlug.value))
const base = computed(() => `/o/${route.params.org}/projects/${projectSlug.value}`)

const items = computed<NavItem[]>(() => [
  { key: 'agents', label: t('nav.agents'), icon: 'lucide:bot', active: route.path === base.value },
  { key: 'settings', label: t('nav.settings'), icon: 'lucide:settings', active: route.path.startsWith(`${base.value}/settings`) },
])

function onSelect(key: string) {
  return navigateTo(key === 'settings' ? `${base.value}/settings` : base.value)
}
</script>

<template>
  <SidebarNav :label="project?.name ?? ''" :items="items" @select="onSelect">
    <template #back>
      <NuxtLink :to="`/o/${route.params.org}/projects`" class="inline-flex items-center gap-1 text-xs text-base-content/60 transition-colors hover:text-base-content">
        <Icon name="lucide:chevron-left" size="14" />
        {{ t('project.all') }}
      </NuxtLink>
    </template>
    <template v-if="appVersion" #footer>
      oagent v{{ appVersion }}
    </template>
  </SidebarNav>
</template>
