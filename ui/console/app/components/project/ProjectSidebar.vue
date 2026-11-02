<script setup lang="ts">
import type { NavGroup } from '#base/components/ui/types';
import SidebarNav from '#base/components/ui/SidebarNav.vue';
import SidebarUser from '~/components/layout/SidebarUser.vue';
import { useOrgNav } from '~/components/layout/useOrgNav';
import OrgRoleBadge from '~/components/org/OrgRoleBadge.vue';
import OrgSelector from '~/components/org/OrgSelector.vue';
import ProjectSelector from '~/components/project/ProjectSelector.vue';
import { useOrgsStore } from '~/stores/orgs';
import { useProjectsStore } from '~/stores/projects';

const route = useRoute();
const { t } = useI18n();
const orgs = useOrgsStore();
const projects = useProjectsStore();

const org = computed(() => orgs.bySlug(route.params.org as string));
const project = computed(() => projects.items.find(p => p.orgId === org.value?.id && p.slug === route.params.project));
const base = computed(() => `/o/${route.params.org}/projects/${route.params.project}`);
const orgNav = useOrgNav();

const groups = computed<NavGroup[]>(() => [
  {
    items: [
      { key: 'overview', label: t('nav.overview'), to: base.value, icon: 'lucide:activity', active: route.path === base.value },
      { key: 'agents', label: t('nav.agents'), to: `${base.value}/agents`, icon: 'lucide:bot', active: route.path.startsWith(`${base.value}/agents`) },
      { key: 'webhooks', label: t('webhooks.title'), to: `${base.value}/webhooks`, icon: 'lucide:webhook', active: route.path.startsWith(`${base.value}/webhooks`) },
      { key: 'settings', label: t('nav.settings'), to: `${base.value}/settings`, icon: 'lucide:settings-2', active: route.path.startsWith(`${base.value}/settings`) },
    ],
  },
]);
</script>

<template>
  <SidebarNav :label="project?.name ?? ''" :lead="[orgNav]" :groups="groups">
    <template #top>
      <div class="flex flex-wrap items-center gap-1">
        <OrgSelector />
        <OrgRoleBadge />
      </div>
    </template>
    <template #between>
      <ProjectSelector />
    </template>
    <template #footer>
      <SidebarUser />
    </template>
  </SidebarNav>
</template>
