<script setup lang="ts">
import AppShell from '#base/components/ui/AppShell.vue';
import AgentSidebar from '~/components/agent/AgentSidebar.vue';
import AppSidebar from '~/components/layout/AppSidebar.vue';
import { environmentLabelKey, showsBanner } from '~/components/project/environment';
import ProjectSidebar from '~/components/project/ProjectSidebar.vue';
import { useOrgsStore } from '~/stores/orgs';
import { useProjectsStore } from '~/stores/projects';
import { useUIStore } from '~/stores/ui';

definePageMeta({ middleware: ['active-org'] });

const route = useRoute();
const { t } = useI18n();
const orgs = useOrgsStore();
const projects = useProjectsStore();
const ui = useUIStore();
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
const project = computed(() => (projectSlug.value ? projects.forOrg(org.value!.id).find(p => p.slug === projectSlug.value) : undefined));

watch(() => route.fullPath, () => {
  ui.navOpen = false;
});

await projects.ensureLoaded(org.value.id);
</script>

<template>
  <AppShell v-model:open="ui.navOpen" :close-label="t('nav.close_menu')">
    <main class="min-w-0 flex-1 p-4 sm:p-6">
      <p v-if="showsBanner(project?.environment)" role="note" class="mb-4 alert alert-info alert-soft text-sm" data-testid="environment-banner">
        <Icon name="lucide:flask-conical" size="16" />
        {{ t('environment.banner', { env: t(environmentLabelKey(project?.environment)) }) }}
      </p>
      <NuxtPage />
    </main>
    <template #sidebar>
      <AgentSidebar v-if="agentSlug" />
      <ProjectSidebar v-else-if="projectSlug" />
      <AppSidebar v-else />
    </template>
  </AppShell>
</template>
