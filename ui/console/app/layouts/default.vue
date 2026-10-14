<script setup lang="ts">
import AppNavbar from '~/components/layout/AppNavbar.vue';
import OrgCreateModal from '~/components/org/create/OrgCreateModal.vue';
import ProjectCreateModal from '~/components/project/create/ProjectCreateModal.vue';
import { useOrgsStore } from '~/stores/orgs';

const route = useRoute();
const orgs = useOrgsStore();

const orgId = computed(() => {
  const slug = route.params.org as string | undefined;
  return (slug ? orgs.bySlug(slug)?.id : '') ?? '';
});
</script>

<template>
  <div class="flex min-h-dvh flex-col bg-base-100 text-base-content">
    <AppNavbar />
    <slot />
    <OrgCreateModal />
    <ProjectCreateModal v-if="orgId" :org-id="orgId" />
  </div>
</template>
