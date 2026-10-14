<script setup lang="ts">
import type { NavItem } from '~/components/ui/types';
import SidebarNav from '~/components/ui/SidebarNav.vue';

const route = useRoute();
const { t } = useI18n();
const base = computed(() => `/o/${route.params.org}/settings`);

const items = computed<NavItem[]>(() => [
  { key: 'general', label: t('nav.general'), icon: 'lucide:sliders-horizontal', active: route.path === base.value },
  { key: 'members', label: t('members.title'), icon: 'lucide:users', active: route.path.startsWith(`${base.value}/members`) },
  { key: 'api-keys', label: t('apikeys.title'), icon: 'lucide:key-round', active: route.path.startsWith(`${base.value}/api-keys`) },
  { key: 'personal-keys', label: t('apikeys.personal_title'), icon: 'lucide:user-round-key', active: route.path.startsWith(`${base.value}/personal-keys`) },
]);

function onSelect(key: string) {
  return navigateTo(key === 'general' ? base.value : `${base.value}/${key}`);
}
</script>

<template>
  <SidebarNav :label="t('settings.title')" :items="items" @select="onSelect" />
</template>
