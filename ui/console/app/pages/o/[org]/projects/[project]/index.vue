<script setup lang="ts">
import type { SetupProgress } from '~/components/project/setupSteps';
import type { Agent } from '~/stores/agents';
import AppBadge from '#base/components/ui/AppBadge.vue';
import CopyButton from '#base/components/ui/CopyButton.vue';
import LoadError from '#base/components/ui/LoadError.vue';
import PageHeader from '#base/components/ui/PageHeader.vue';
import Section from '#base/components/ui/Section.vue';
import SetupChecklist from '~/components/project/SetupChecklist.vue';
import { setupSteps } from '~/components/project/setupSteps';
import { useAgentRuntimeStore } from '~/stores/agentRuntime';
import { useAgentsStore } from '~/stores/agents';
import { useChannelsStore } from '~/stores/channels';
import { useMcpServersStore } from '~/stores/mcpServers';
import { useOrgsStore } from '~/stores/orgs';
import { useProjectsStore } from '~/stores/projects';
import { useProvidersStore } from '~/stores/providers';
import { useTryItStore } from '~/stores/tryIt';

const route = useRoute();
const { t } = useI18n();
const orgs = useOrgsStore();
const projects = useProjectsStore();
const agents = useAgentsStore();
const providers = useProvidersStore();
const mcpServers = useMcpServersStore();

const org = computed(() => orgs.bySlug(route.params.org as string));
const project = computed(() =>
  projects.items.find(item => item.orgId === org.value?.id && item.slug === route.params.project),
);

if (!project.value) {
  throw createError({ statusCode: 404, statusMessage: t('error.project_not_found'), fatal: true });
}

await Promise.all([agents.ensureLoaded(project.value.id), providers.ensureLoaded(project.value.id), mcpServers.ensureLoaded(project.value.id)]);

const items = computed(() => (project.value ? agents.forProject(project.value.id) : []));
const runtime = useAgentRuntimeStore();
const tryIt = useTryItStore();
const channels = useChannelsStore();
function progressOf(agent: Agent | undefined): SetupProgress {
  if (!agent) {
    return { agentConfigured: false, tried: false, published: false, whatsappConnected: false };
  }
  return {
    agentConfigured: !!runtime.draftFor(agent.id, agent.slug).providerId,
    tried: tryIt.tried.includes(agent.id),
    published: !!agent.publishedVersion && !agent.unpublishedChanges,
    whatsappConnected: channels.list(agent.id).some(c => c.status === 'connected'),
  };
}

const steps = computed(() => setupSteps({
  ...progressOf(items.value[0]),
  base: `/o/${route.params.org}/projects/${route.params.project}`,
  agentSlug: items.value[0]?.slug,
  providers: providers.items.length,
  mcpServers: mcpServers.items.length,
}));

useHead({ title: () => `${project.value?.name ?? t('project.title')} · OAgent` });
</script>

<template>
  <div v-if="project" class="flex flex-col gap-5">
    <PageHeader :title="project.name" :description="t('project.overview_subtitle')">
      <template #status>
        <p class="flex items-center gap-1 text-xs text-base-content/70">
          <span class="font-mono">{{ project.id }}</span>
          <CopyButton :value="project.id" :label="t('common.copy_label', { label: t('common.id') })" />
        </p>
      </template>
    </PageHeader>

    <SetupChecklist :steps="steps" />

    <LoadError v-if="agents.error" :error="agents.error" :busy="agents.loading" :retry-label="t('common.retry')" @retry="agents.load(project.id)" />
    <Section v-else :title="t('nav.agents')" flush class="max-w-2xl" data-testid="project-agents">
      <ul v-if="items.length" class="divide-y divide-base-300">
        <li v-for="agent in items" :key="agent.id" class="flex items-center gap-2 px-4 py-2 text-ui">
          <div class="min-w-0 flex-1">
            <div class="flex min-w-0 items-center gap-2">
              <NuxtLink :to="`/o/${org?.slug}/projects/${project.slug}/agents/${agent.slug}`" class="link link-hover min-w-0 truncate font-medium">
                {{ agent.name }}
              </NuxtLink>
              <span class="min-w-0 truncate font-mono text-xs text-base-content/70">{{ agent.slug }}</span>
            </div>
            <p v-if="agent.description" class="truncate text-xs text-base-content/70">
              {{ agent.description }}
            </p>
          </div>
          <AppBadge v-if="agent.unpublishedChanges" :label="t('agent.draft_changes')" outline class="shrink-0" />
        </li>
      </ul>
      <p v-else class="px-4 py-3 text-ui text-base-content/70">
        {{ t('setup.agents_first') }}
        <NuxtLink to="#setup" class="link">
          {{ t('setup.see_checklist') }}
        </NuxtLink>
      </p>
    </Section>
  </div>
</template>
