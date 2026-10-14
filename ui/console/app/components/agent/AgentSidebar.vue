<script setup lang="ts">
import type { NavItem } from '~/components/ui/types';
import SidebarNav from '~/components/ui/SidebarNav.vue';
import { useAgentsStore } from '~/stores/agents';
import { useOrgsStore } from '~/stores/orgs';
import { useProjectsStore } from '~/stores/projects';

const route = useRoute();
const { t } = useI18n();
const { appVersion } = useRuntimeConfig().public;
const orgs = useOrgsStore();
const projects = useProjectsStore();
const agents = useAgentsStore();

const org = computed(() => orgs.bySlug(route.params.org as string));
const project = computed(() => projects.items.find(p => p.orgId === org.value?.id && p.slug === route.params.project));
const agentSlug = computed(() => route.params.agent as string);
const agent = computed(() => (project.value ? agents.items.find(a => a.projectId === project.value!.id && a.slug === agentSlug.value) : undefined));
const base = computed(() => `/o/${route.params.org}/projects/${route.params.project}/agents/${agentSlug.value}`);

watch(project, (value) => {
  if (value) {
    agents.ensureLoaded(value.id);
  }
}, { immediate: true });

const items = computed<NavItem[]>(() => [
  { key: 'general', label: t('nav.general'), icon: 'lucide:info', active: route.path === base.value },
  { key: 'prompt', label: t('nav.prompt'), icon: 'lucide:file-text', active: route.path === `${base.value}/prompt` },
  { key: 'model', label: t('nav.model'), icon: 'lucide:cpu', active: route.path === `${base.value}/model` },
  { key: 'tools', label: t('nav.tools'), icon: 'lucide:hammer', active: route.path === `${base.value}/tools` },
]);

function onSelect(key: string) {
  return navigateTo(key === 'general' ? base.value : `${base.value}/${key}`);
}
</script>

<template>
  <SidebarNav :label="agent?.name ?? agentSlug" :items="items" :version="appVersion" @select="onSelect">
    <template #back>
      <NuxtLink :to="`/o/${route.params.org}/projects/${route.params.project}/agents`" class="inline-flex items-center gap-1 text-xs text-base-content/60 transition-colors hover:text-base-content">
        <Icon name="lucide:chevron-left" size="14" />
        {{ t('agent.all') }}
      </NuxtLink>
    </template>
  </SidebarNav>
</template>
