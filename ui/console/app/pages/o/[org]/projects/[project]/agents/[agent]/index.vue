<script setup lang="ts">
import type { AgentConfig } from '~/preview/samples';
import CopyButton from '#base/components/ui/CopyButton.vue';
import PageHeader from '#base/components/ui/PageHeader.vue';
import Section from '#base/components/ui/Section.vue';
import AgentApiExample from '~/components/agent/configure/AgentApiExample.vue';
import AgentConfigForm from '~/components/agent/configure/AgentConfigForm.vue';
import { sampleConfig, sampleModels, sampleTools } from '~/preview/samples';
import { useAgentsStore } from '~/stores/agents';
import { useOrgsStore } from '~/stores/orgs';
import { useProjectsStore } from '~/stores/projects';

const { t } = useI18n();
const route = useRoute();
const orgs = useOrgsStore();
const projects = useProjectsStore();
const agents = useAgentsStore();

const org = computed(() => orgs.bySlug(route.params.org as string));
const project = computed(() =>
  projects.items.find(item => item.orgId === org.value?.id && item.slug === route.params.project),
);

if (!project.value) {
  throw createError({ statusCode: 404, statusMessage: t('error.project_not_found'), fatal: true });
}

await agents.ensureLoaded(project.value.id);

const agent = computed(() => agents.items.find(a => a.projectId === project.value?.id && a.slug === route.params.agent));

if (!agent.value) {
  throw createError({ statusCode: 404, statusMessage: t('error.agent_not_found'), fatal: true });
}

// TODO: load agent.draft and autosave through UpdateAgentDraft once the agent.v1 versions land; Publish then opens the diff modal.
const config = ref<AgentConfig>(sampleConfig());
const reasonId = useId();
const list = computed(() => `/o/${route.params.org}/projects/${route.params.project}/agents`);

useHead({ title: () => `${agent.value?.name ?? ''} · ${t('agent.configure')} · OAgent` });
</script>

<template>
  <div v-if="agent" class="space-y-6">
    <PageHeader
      :title="agent.name"
      :description="agent.description"
      :preview="t('preview.badge')"
      :crumbs="[{ label: t('nav.agents'), to: list }, { label: agent.name }]"
      :crumbs-label="t('common.breadcrumb')"
    >
      <template #status>
        <p :id="reasonId" class="flex items-center gap-1.5 text-sm text-base-content/70">
          <Icon name="lucide:info" size="14" class="shrink-0 text-info" />
          {{ t('configure.not_saved') }}
        </p>
        <p class="flex items-center gap-1 text-xs text-base-content/70">
          <span class="font-mono">{{ agent.id }}</span>
          <CopyButton :value="agent.id" :label="t('common.copy')" />
        </p>
      </template>
      <template #actions>
        <button type="button" class="btn btn-ghost btn-sm" disabled :aria-describedby="reasonId">
          {{ t('configure.view_changes') }}
        </button>
        <button type="button" class="btn btn-primary btn-sm" disabled :aria-describedby="reasonId">
          {{ t('configure.publish') }}
        </button>
      </template>
    </PageHeader>

    <div class="grid gap-6 lg:grid-cols-[minmax(0,1fr)_24rem]">
      <AgentConfigForm v-model="config" :models="sampleModels" :tools="sampleTools" />
      <div class="space-y-6 lg:sticky lg:top-20 lg:self-start">
        <Section :title="t('configure.test_title')" :description="t('configure.test_hint')">
          <form class="join w-full" @submit.prevent>
            <label class="sr-only" :for="`${reasonId}-msg`">{{ t('configure.test_message') }}</label>
            <input :id="`${reasonId}-msg`" type="text" class="input join-item w-full" :placeholder="t('configure.test_message')" disabled>
            <button type="submit" class="btn join-item" disabled>
              {{ t('configure.send') }}
            </button>
          </form>
        </Section>
        <AgentApiExample :agent-id="agent.id" :config="config" />
      </div>
    </div>
  </div>
</template>
