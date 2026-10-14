<script setup lang="ts">
import AgentSidebar from '~/components/agent/AgentSidebar.vue';
import AppSidebar from '~/components/layout/AppSidebar.vue';
import SettingsSidebar from '~/components/org/SettingsSidebar.vue';
import ProjectSidebar from '~/components/project/ProjectSidebar.vue';
import { useOrgsStore } from '~/stores/orgs';
import { useProjectsStore } from '~/stores/projects';
import { useUIStore } from '~/stores/ui';

const route = useRoute();
const { t } = useI18n();
const orgs = useOrgsStore();
const projects = useProjectsStore();
const ui = useUIStore();
const { appVersion } = useRuntimeConfig().public;
await orgs.ensureLoaded();

if (orgs.error) {
  throw createError({ statusCode: 502, statusMessage: t('error.load_orgs'), fatal: true });
}

const org = computed(() => orgs.bySlug(route.params.org as string));

if (!org.value) {
  throw createError({ statusCode: 404, statusMessage: t('error.org_not_found'), fatal: true });
}

const projectSlug = computed(() => route.params.project as string | undefined);
const agentSlug = computed(() => route.params.agent as string | undefined);
const inSettings = computed(() => route.path.startsWith(`/o/${route.params.org}/settings`));

watch(() => route.fullPath, () => {
  ui.navOpen = false;
});

await projects.ensureLoaded(org.value.id);
</script>

<template>
  <div class="flex flex-1">
    <div v-if="ui.navOpen" class="fixed inset-x-0 top-14 bottom-0 z-40 bg-black/40 md:hidden" @click="ui.navOpen = false" />
    <div
      class="z-40"
      :class="ui.navOpen
        ? 'fixed top-14 bottom-0 left-0 flex flex-col overflow-y-auto bg-base-100 max-md:[&_[data-sidebar-footer]]:hidden shadow-xl md:static md:z-auto md:flex-row md:overflow-visible md:bg-transparent md:shadow-none'
        : 'hidden md:flex md:flex-row'"
    >
      <AgentSidebar v-if="agentSlug" />
      <ProjectSidebar v-else-if="projectSlug" />
      <template v-else>
        <AppSidebar />
        <SettingsSidebar v-if="inSettings" />
      </template>
      <div v-if="appVersion" class="flex flex-1 items-end border-r border-base-300 bg-base-200/40 px-4 py-3 text-xs text-base-content/40 md:hidden">
        oagent v{{ appVersion }}
      </div>
    </div>
    <main class="min-w-0 flex-1 p-4 sm:p-6">
      <NuxtPage />
    </main>
  </div>
</template>
