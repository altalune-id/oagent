<script setup lang="ts">
import type { NavItem } from '#base/components/ui/types';
import SidebarNav from '#base/components/ui/SidebarNav.vue';
import { useOrgsStore } from '~/stores/orgs';

const route = useRoute();
const { t } = useI18n();
const { appVersion } = useRuntimeConfig().public;
const orgs = useOrgsStore();
const org = computed(() => route.params.org as string);
const label = computed(() => orgs.bySlug(org.value)?.name ?? '');

const targets: Record<string, string> = {
  overview: '',
  projects: '/projects',
  settings: '/settings',
};

const items = computed<NavItem[]>(() => [
  { key: 'overview', label: t('nav.overview'), icon: 'lucide:layout-dashboard', active: route.path === `/o/${org.value}` },
  { key: 'projects', label: t('nav.projects'), icon: 'lucide:folder', active: route.path.startsWith(`/o/${org.value}/projects`) },
  { key: 'settings', label: t('nav.settings'), icon: 'lucide:settings', active: route.path.startsWith(`/o/${org.value}/settings`) },
]);

function onSelect(key: string) {
  return navigateTo(`/o/${org.value}${targets[key] ?? ''}`);
}
</script>

<template>
  <SidebarNav :label="label" :items="items" :version="appVersion" @select="onSelect">
    <template #back>
      <NuxtLink to="/orgs" class="inline-flex items-center gap-1 text-xs text-base-content/60 transition-colors hover:text-base-content">
        <Icon name="lucide:chevron-left" size="14" />
        {{ t('org.all') }}
      </NuxtLink>
    </template>
  </SidebarNav>
</template>
