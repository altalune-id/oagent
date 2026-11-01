<script setup lang="ts">
import type { ToolInfo } from '~/preview/samples';
import AppModal from '#base/components/ui/AppModal.vue';
import CopyButton from '#base/components/ui/CopyButton.vue';
import ErrorText from '#base/components/ui/ErrorText.vue';
import LoadError from '#base/components/ui/LoadError.vue';
import PageHeader from '#base/components/ui/PageHeader.vue';
import Section from '#base/components/ui/Section.vue';
import { useToast } from '#base/components/ui/toast';
import { useDates } from '#base/composables/useDates';
import AgentApiExample from '~/components/agent/configure/AgentApiExample.vue';
import AgentConfigForm from '~/components/agent/configure/AgentConfigForm.vue';
import { useAgentDraft } from '~/components/agent/configure/useAgentDraft';
import { useCan } from '~/composables/useCan';
import { sampleModels, sampleTools } from '~/preview/samples';
import { useAgentsStore } from '~/stores/agents';
import { useAuthStore } from '~/stores/auth';
import { useOrgsStore } from '~/stores/orgs';
import { useProjectsStore } from '~/stores/projects';

const { t } = useI18n();
const route = useRoute();
const orgs = useOrgsStore();
const projects = useProjectsStore();
const agents = useAgentsStore();
const canEdit = useCan('agent:write');
const toast = useToast();
const { when } = useDates();

const org = computed(() => orgs.bySlug(route.params.org as string));
const project = computed(() =>
  projects.items.find(item => item.orgId === org.value?.id && item.slug === route.params.project),
);

if (!project.value) {
  throw createError({ statusCode: 404, statusMessage: t('error.project_not_found'), fatal: true });
}

await Promise.all([agents.ensureLoaded(project.value.id), useAuthStore().load()]);

const agent = computed(() => agents.items.find(a => a.projectId === project.value?.id && a.slug === route.params.agent));

if (!agent.value) {
  throw createError({ statusCode: 404, statusMessage: t('error.agent_not_found'), fatal: true });
}

const draft = useAgentDraft(agent.value.id);
await draft.load();

const { config, state, diff } = draft;
const reasonId = useId();
const publishOpen = ref(false);
const list = computed(() => `/o/${route.params.org}/projects/${route.params.project}/agents`);
const current = computed(() => state.value?.publishedVersion ?? 0);
const models = computed(() => [...new Set([...sampleModels, config.value.model].filter(Boolean))]);
const tools = computed<ToolInfo[]>(() => [
  ...sampleTools,
  ...config.value.tools.filter(tool => !sampleTools.some(s => s.name === tool.name)).map(tool => ({ name: tool.name, description: '' })),
]);
const canPublish = computed(() => canEdit.value && !draft.publishing.value && (current.value === 0 || !!state.value?.unpublishedChanges || draft.dirty.value));

const status = computed(() => {
  if (draft.saving.value) {
    return t('configure.saving');
  }
  if (draft.dirty.value) {
    return t('configure.unsaved');
  }
  return state.value?.draftUpdatedAt ? t('configure.saved_at', { when: when(state.value.draftUpdatedAt) }) : '';
});
const liveLine = computed(() => (current.value ? t('configure.live_version', { n: current.value }) : t('configure.never_published')));

async function publish() {
  const res = await draft.publish();
  if (!res) {
    return;
  }
  publishOpen.value = false;
  toast.push(res.created ? 'success' : 'info', t(res.created ? 'configure.published' : 'configure.nothing_changed', { n: res.version }));
}

useHead({ title: () => `${agent.value?.name ?? ''} · ${t('agent.configure')} · OAgent` });
</script>

<template>
  <div v-if="agent" class="space-y-6">
    <PageHeader
      :title="agent.name"
      :description="agent.description"
      :crumbs="[{ label: t('nav.agents'), to: list }, { label: agent.name }]"
      :crumbs-label="t('common.breadcrumb')"
    >
      <template #status>
        <p :id="reasonId" class="text-sm text-base-content/70">
          <span role="status">{{ status }}</span>
          {{ liveLine }}
        </p>
        <p class="flex items-center gap-1 text-xs text-base-content/70">
          <span class="font-mono">{{ agent.id }}</span>
          <CopyButton :value="agent.id" :label="t('common.copy')" />
        </p>
      </template>
      <template v-if="canEdit" #actions>
        <button type="button" class="btn btn-primary btn-sm" :disabled="!canPublish" :aria-describedby="reasonId" @click="publishOpen = true">
          {{ t('configure.publish') }}
        </button>
      </template>
    </PageHeader>

    <LoadError :error="draft.loadError.value" :busy="false" :retry-label="t('common.retry')" @retry="draft.load()" />
    <ErrorText :error="draft.saveError.value" />

    <div class="grid gap-6 lg:grid-cols-[minmax(0,1fr)_24rem]">
      <AgentConfigForm v-model="config" :models="models" :tools="tools" :disabled="!canEdit" />
      <div class="space-y-6 lg:sticky lg:top-20 lg:self-start">
        <Section :title="t('configure.test_title')" :description="t('configure.test_hint')">
          <template #actions>
            <span class="badge badge-info badge-soft badge-sm">{{ t('preview.badge') }}</span>
          </template>
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

    <AppModal :open="publishOpen" :title="t('configure.publish_title', { name: agent.name, n: current + 1 })" :dismissible="!draft.publishing.value" @close="publishOpen = false">
      <p class="text-sm">
        {{ current ? t('configure.publish_body', { n: current + 1, current }) : t('configure.publish_first') }}
      </p>
      <ul v-if="diff.length" class="mt-3 list-disc space-y-1 pl-5 text-sm">
        <li v-for="(change, i) in diff" :key="i">
          {{ t(change.key, change.params ?? {}) }}
        </li>
      </ul>
      <p v-else class="mt-3 text-sm text-base-content/70">
        {{ t('configure.no_changes') }}
      </p>
      <ErrorText class="mt-3" :error="draft.publishError.value" />
      <template #footer>
        <button type="button" class="btn btn-ghost btn-sm" :disabled="draft.publishing.value" @click="publishOpen = false">
          {{ t('configure.keep_editing') }}
        </button>
        <button type="button" class="btn btn-primary btn-sm" :disabled="draft.publishing.value" :aria-busy="draft.publishing.value || undefined" @click="publish()">
          <span v-if="draft.publishing.value" class="loading loading-spinner loading-xs" aria-hidden="true" />
          {{ draft.publishing.value ? t('configure.publishing') : t('configure.publish_n', { n: current + 1 }) }}
        </button>
      </template>
    </AppModal>
  </div>
</template>
