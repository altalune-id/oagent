<script setup lang="ts">
import SidebarNav from '#base/components/ui/SidebarNav.vue';
import SidebarUser from '~/components/layout/SidebarUser.vue';
import { useOrgNav } from '~/components/layout/useOrgNav';
import OrgRoleBadge from '~/components/org/OrgRoleBadge.vue';
import OrgSelector from '~/components/org/OrgSelector.vue';
import ProjectSelector from '~/components/project/ProjectSelector.vue';
import { useProjectNav } from '~/components/project/useProjectNav';
import { useOrgsStore } from '~/stores/orgs';
import { useProjectsStore } from '~/stores/projects';

const route = useRoute();
const orgs = useOrgsStore();
const projects = useProjectsStore();

const org = computed(() => orgs.bySlug(route.params.org as string));
const project = computed(() => projects.items.find(p => p.orgId === org.value?.id && p.slug === route.params.project));
const orgNav = useOrgNav();

const groups = useProjectNav();
</script>

<template>
  <SidebarNav :label="project?.name ?? ''" :lead="[orgNav]" :groups="groups">
    <template #top>
      <div class="flex items-center gap-1">
        <div class="min-w-0 flex-1">
          <OrgSelector />
        </div>
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
