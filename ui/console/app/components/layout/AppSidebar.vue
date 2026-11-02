<script setup lang="ts">
import SidebarNav from '#base/components/ui/SidebarNav.vue';
import SidebarUser from '~/components/layout/SidebarUser.vue';
import { useOrgNav } from '~/components/layout/useOrgNav';
import OrgRoleBadge from '~/components/org/OrgRoleBadge.vue';
import OrgSelector from '~/components/org/OrgSelector.vue';
import { useOrgsStore } from '~/stores/orgs';

const route = useRoute();
const orgs = useOrgsStore();
const label = computed(() => orgs.bySlug(route.params.org as string)?.name ?? '');
const orgNav = useOrgNav();
</script>

<template>
  <SidebarNav :label="label" :groups="[orgNav]">
    <template #top>
      <div class="flex flex-wrap items-center gap-1">
        <OrgSelector />
        <OrgRoleBadge />
      </div>
    </template>
    <template #footer>
      <SidebarUser />
    </template>
  </SidebarNav>
</template>
