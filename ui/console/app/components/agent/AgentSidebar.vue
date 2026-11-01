<script setup lang="ts">
import type { NavGroup } from '#base/components/ui/types';
import SidebarNav from '#base/components/ui/SidebarNav.vue';
import { useAgentsStore } from '~/stores/agents';
import { useOrgsStore } from '~/stores/orgs';
import { useProjectsStore } from '~/stores/projects';

const route = useRoute();
const { t } = useI18n();
const orgs = useOrgsStore();
const projects = useProjectsStore();
const agents = useAgentsStore();

const org = computed(() => orgs.bySlug(route.params.org as string));
const project = computed(() => projects.items.find(p => p.orgId === org.value?.id && p.slug === route.params.project));
const agentSlug = computed(() => route.params.agent as string);
const agent = computed(() => (project.value ? agents.items.find(a => a.projectId === project.value!.id && a.slug === agentSlug.value) : undefined));
const list = computed(() => `/o/${route.params.org}/projects/${route.params.project}/agents`);
const base = computed(() => `${list.value}/${agentSlug.value}`);

watch(project, (value) => {
  if (value) {
    agents.ensureLoaded(value.id);
  }
}, { immediate: true });

const groups = computed<NavGroup[]>(() => [
  {
    items: [
      { key: 'configure', label: t('agent.configure'), to: base.value, icon: 'lucide:sliders-horizontal', active: route.path === base.value },
      { key: 'runs', label: t('agent.runs'), to: `${base.value}/runs`, icon: 'lucide:list-tree', active: route.path.startsWith(`${base.value}/runs`) },
    ],
  },
]);
</script>

<template>
  <SidebarNav :label="agent?.name ?? agentSlug" :groups="groups">
    <template #back>
      <NuxtLink :to="list" class="inline-flex items-center gap-1 text-sm text-base-content/70 hover:text-base-content">
        <Icon name="lucide:chevron-left" size="14" />
        {{ t('agent.all') }}
      </NuxtLink>
      <p class="mt-3 truncate font-semibold">
        {{ agent?.name ?? agentSlug }}
      </p>
    </template>
  </SidebarNav>
</template>
