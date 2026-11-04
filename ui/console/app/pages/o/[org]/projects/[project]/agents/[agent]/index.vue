<script setup lang="ts">
import type { ToolInfo } from '~/preview/samples';
import AppButton from '#base/components/ui/AppButton.vue';
import AppModal from '#base/components/ui/AppModal.vue';
import ErrorText from '#base/components/ui/ErrorText.vue';
import LoadError from '#base/components/ui/LoadError.vue';
import { useToast } from '#base/components/ui/toast';
import { useDates } from '#base/composables/useDates';
import AgentConfigForm from '~/components/agent/configure/AgentConfigForm.vue';
import ConfigurePane from '~/components/agent/configure/ConfigurePane.vue';
import { useAgentDraft } from '~/components/agent/configure/useAgentDraft';
import { useCan } from '~/composables/useCan';
import { sampleModels, sampleTools } from '~/preview/samples';
import { useAgentsStore } from '~/stores/agents';
import { useAuthStore } from '~/stores/auth';
import { useOrgsStore } from '~/stores/orgs';
import { useProjectsStore } from '~/stores/projects';

definePageMeta({ bleed: true });

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

if (agents.error) {
  throw createError({ statusCode: 502, statusMessage: t('error.load_agents'), fatal: true });
}

const agent = computed(() => agents.items.find(a => a.projectId === project.value?.id && a.slug === route.params.agent));

if (!agent.value) {
  throw createError({ statusCode: 404, statusMessage: t('error.agent_not_found'), fatal: true });
}

const draft = useAgentDraft(agent.value.id);
await draft.load();

const { config, state, diff } = draft;
const reasonId = useId();
const publishOpen = ref(false);
const current = computed(() => state.value?.publishedVersion ?? 0);
const models = computed(() => [...new Set([...sampleModels, config.value.model].filter(Boolean))]);
const tools = computed<ToolInfo[]>(() => [
  ...sampleTools,
  ...config.value.tools.filter(tool => !sampleTools.some(s => s.name === tool.name)).map(tool => ({ name: tool.name, description: '' })),
]);
const usesSamples = computed(() => sampleModels.includes(config.value.model) || config.value.tools.some(tool => sampleTools.some(s => s.name === tool.name)));
const loaded = computed(() => !!state.value && !draft.loadError.value);
const canPublish = computed(() => canEdit.value && loaded.value && !usesSamples.value && !draft.publishing.value && (current.value === 0 || !!state.value?.unpublishedChanges || draft.dirty.value));

onBeforeRouteLeave(() => draft.leaveGuard());
onBeforeRouteUpdate(() => draft.leaveGuard());
onMounted(() => window.addEventListener('beforeunload', draft.warnUnload));
onBeforeUnmount(() => window.removeEventListener('beforeunload', draft.warnUnload));

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

function changeIcon(key: string) {
  if (key.endsWith('_removed')) {
    return 'lucide:minus';
  }
  return key === 'configure.change_first' || key.includes('_added') ? 'lucide:plus' : 'lucide:pencil';
}

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
  <div v-if="agent" class="flex min-w-0 flex-col">
    <div class="flex flex-wrap items-center justify-between gap-3 border-b border-base-300 px-4 py-3 lg:px-6">
      <div class="min-w-0">
        <h1 class="truncate text-xl font-semibold">
          {{ agent.name }}
        </h1>
        <p :id="reasonId" class="text-ui text-base-content/70">
          <span role="status">{{ status }}</span>
          {{ liveLine }}
          <template v-if="canEdit && loaded && usesSamples">
            {{ t('configure.samples_block') }}
          </template>
        </p>
      </div>
      <div v-if="canEdit" class="flex items-center gap-2">
        <AppButton variant="plain" size="sm" :disabled="!loaded" @click="publishOpen = true">
          <Icon name="lucide:git-compare" size="16" aria-hidden="true" />
          {{ t('configure.view_changes') }}
        </AppButton>
        <AppButton size="sm" :disabled="!canPublish" :aria-describedby="reasonId" @click="publishOpen = true">
          <Icon name="lucide:upload" size="16" aria-hidden="true" />
          {{ t('configure.publish') }}
        </AppButton>
      </div>
    </div>

    <div class="px-4 lg:px-6">
      <LoadError :error="draft.loadError.value" :busy="false" :retry-label="t('common.retry')" @retry="draft.load()" />
      <ErrorText :error="draft.saveError.value" />
    </div>

    <div class="grid min-w-0 grid-cols-[minmax(0,1fr)] xl:grid-cols-[minmax(0,1fr)_minmax(22rem,26rem)]">
      <AgentConfigForm v-model="config" :models="models" :tools="tools" :disabled="!canEdit || !loaded" />
      <ConfigurePane :agent-id="agent.id" :config="config" />
    </div>

    <AppModal :open="publishOpen" :title="t('configure.publish_title', { name: agent.name, n: current + 1 })" :dismissible="!draft.publishing.value" @close="publishOpen = false">
      <p class="text-ui text-base-content/70">
        {{ current ? t('configure.publish_body', { n: current + 1, current }) : t('configure.publish_first') }}
      </p>
      <ul v-if="diff.length" class="flex flex-col gap-2 py-3 text-ui">
        <li v-for="(change, i) in diff" :key="i" class="flex gap-2">
          <Icon :name="changeIcon(change.key)" size="16" class="mt-0.5 shrink-0" aria-hidden="true" />
          <span>{{ t(change.key, change.params ?? {}) }}</span>
        </li>
      </ul>
      <p v-else class="py-3 text-ui text-base-content/70">
        {{ t('configure.no_changes') }}
      </p>
      <ErrorText :error="draft.publishError.value" />
      <template #footer>
        <AppButton variant="plain" size="sm" :disabled="draft.publishing.value" @click="publishOpen = false">
          {{ t('configure.keep_editing') }}
        </AppButton>
        <AppButton size="sm" :busy="draft.publishing.value" :aria-busy="draft.publishing.value || undefined" :disabled="!canPublish" :aria-describedby="reasonId" @click="publish()">
          {{ draft.publishing.value ? t('configure.publishing') : t('configure.publish_n', { n: current + 1 }) }}
        </AppButton>
      </template>
    </AppModal>
  </div>
</template>
