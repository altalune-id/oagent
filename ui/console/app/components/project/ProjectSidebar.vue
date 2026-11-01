<script setup lang="ts">
import type { NavGroup } from '#base/components/ui/types';
import SidebarNav from '#base/components/ui/SidebarNav.vue';
import OrgSelector from '~/components/org/OrgSelector.vue';
import ProjectSelector from '~/components/project/ProjectSelector.vue';
import { useOrgsStore } from '~/stores/orgs';
import { useProjectsStore } from '~/stores/projects';

const route = useRoute();
const { t } = useI18n();
const { appVersion } = useRuntimeConfig().public;
const orgs = useOrgsStore();
const projects = useProjectsStore();

const org = computed(() => orgs.bySlug(route.params.org as string));
const project = computed(() => projects.items.find(p => p.orgId === org.value?.id && p.slug === route.params.project));
const base = computed(() => `/o/${route.params.org}/projects/${route.params.project}`);

const groups = computed<NavGroup[]>(() => [
  {
    items: [
      { key: 'overview', label: t('nav.overview'), to: base.value, icon: 'lucide:layout-dashboard', active: route.path === base.value },
      { key: 'agents', label: t('nav.agents'), to: `${base.value}/agents`, icon: 'lucide:bot', active: route.path.startsWith(`${base.value}/agents`) },
      { key: 'webhooks', label: t('webhooks.title'), to: `${base.value}/webhooks`, icon: 'lucide:webhook', active: route.path.startsWith(`${base.value}/webhooks`) },
      { key: 'settings', label: t('nav.settings'), to: `${base.value}/settings`, icon: 'lucide:settings', active: route.path.startsWith(`${base.value}/settings`) },
    ],
  },
]);
</script>

<template>
  <SidebarNav :label="project?.name ?? ''" :groups="groups">
    <template #top>
      <div class="flex flex-col items-start gap-1">
        <OrgSelector />
        <ProjectSelector />
      </div>
    </template>
    <template v-if="appVersion" #footer>
      OAgent v{{ appVersion }}
    </template>
  </SidebarNav>
</template>
