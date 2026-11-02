<script setup lang="ts">
import type { NavGroup } from '#base/components/ui/types';
import SidebarNav from '#base/components/ui/SidebarNav.vue';
import OrgRoleBadge from '~/components/org/OrgRoleBadge.vue';
import OrgSelector from '~/components/org/OrgSelector.vue';
import { useOrgsStore } from '~/stores/orgs';

const route = useRoute();
const { t } = useI18n();
const { appVersion } = useRuntimeConfig().public;
const orgs = useOrgsStore();
const base = computed(() => `/o/${route.params.org}`);
const settings = computed(() => `${base.value}/settings`);
const label = computed(() => orgs.bySlug(route.params.org as string)?.name ?? '');

const groups = computed<NavGroup[]>(() => [
  {
    items: [
      { key: 'overview', label: t('nav.overview'), to: base.value, icon: 'lucide:building-complex', active: route.path === base.value },
      { key: 'projects', label: t('nav.projects'), to: `${base.value}/projects`, icon: 'lucide:folders', active: route.path.startsWith(`${base.value}/projects`) },
    ],
  },
  {
    label: t('settings.title'),
    items: [
      { key: 'general', label: t('nav.general'), to: settings.value, icon: 'lucide:settings', active: route.path === settings.value },
      { key: 'members', label: t('members.title'), to: `${settings.value}/members`, icon: 'lucide:users', active: route.path.startsWith(`${settings.value}/members`) },
      { key: 'api-keys', label: t('apikeys.title'), to: `${settings.value}/api-keys`, icon: 'lucide:key-round', active: route.path.startsWith(`${settings.value}/api-keys`) },
      { key: 'personal-keys', label: t('apikeys.personal_title'), to: `${settings.value}/personal-keys`, icon: 'lucide:user-round-key', active: route.path.startsWith(`${settings.value}/personal-keys`) },
    ],
  },
]);
</script>

<template>
  <SidebarNav :label="label" :groups="groups">
    <template #top>
      <div class="flex flex-wrap items-center gap-1">
        <OrgSelector />
        <OrgRoleBadge />
      </div>
    </template>
    <template v-if="appVersion" #footer>
      <span class="px-2 text-xs text-base-content/70">OAgent v{{ appVersion }}</span>
    </template>
  </SidebarNav>
</template>
