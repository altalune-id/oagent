<script setup lang="ts">
import AppShell from '#base/components/ui/AppShell.vue';
import AgentSidebar from '~/components/agent/AgentSidebar.vue';
import AppSidebar from '~/components/layout/AppSidebar.vue';
import ConsoleTopBar from '~/components/layout/ConsoleTopBar.vue';
import { environmentLabelKey, showsBanner } from '~/components/project/environment';
import ProjectSidebar from '~/components/project/ProjectSidebar.vue';
import { useOrgsStore } from '~/stores/orgs';
import { useProjectsStore } from '~/stores/projects';
import { useRecentStore } from '~/stores/recent';
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

const recent = useRecentStore();
watch([() => route.params.org as string, projectSlug], ([o, p]) => recent.visit(o, p), { immediate: true });

watch(() => route.fullPath, () => {
  ui.navOpen = false;
});

await projects.ensureLoaded(org.value.id);
</script>

<template>
  <AppShell v-model:open="ui.navOpen" :close-label="t('nav.close_menu')" full breakpoint="lg">
    <div v-if="showsBanner(project?.environment)" role="note" class="flex items-center gap-2 bg-warning px-4 py-1 text-ui font-medium text-warning-content" data-testid="environment-banner">
      <Icon name="lucide:flask-conical" size="16" class="shrink-0" aria-hidden="true" />
      <span>{{ t('environment.banner', { env: t(environmentLabelKey(project?.environment)) }) }}</span>
    </div>
    <ConsoleTopBar />
    <main class="min-w-0 flex-1 p-4 lg:p-6">
      <NuxtPage />
    </main>
    <template #sidebar>
      <AgentSidebar v-if="agentSlug" />
      <ProjectSidebar v-else-if="projectSlug" />
      <AppSidebar v-else />
    </template>
  </AppShell>
</template>
