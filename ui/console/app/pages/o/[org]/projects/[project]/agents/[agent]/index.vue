<script setup lang="ts">
import AppButton from '#base/components/ui/AppButton.vue';
import AppModal from '#base/components/ui/AppModal.vue';
import ErrorText from '#base/components/ui/ErrorText.vue';
import LoadError from '#base/components/ui/LoadError.vue';
import { useToast } from '#base/components/ui/toast';
import { useDates } from '#base/composables/useDates';
import AgentConfigForm from '~/components/agent/configure/AgentConfigForm.vue';
import ConfigurePane from '~/components/agent/configure/ConfigurePane.vue';
import { publishBlockers, runtimeChanges } from '~/components/agent/configure/runtime';
import { useAgentDraft } from '~/components/agent/configure/useAgentDraft';
import { useCan } from '~/composables/useCan';
import { useAgentRuntimeStore } from '~/stores/agentRuntime';
import { useAgentsStore } from '~/stores/agents';
import { useAuthStore } from '~/stores/auth';
import { useKnowledgeStore } from '~/stores/knowledge';
import { useMcpServersStore } from '~/stores/mcpServers';
import { useOrgsStore } from '~/stores/orgs';
import { useProjectsStore } from '~/stores/projects';
import { useProvidersStore } from '~/stores/providers';
import { useWebSearchStore } from '~/stores/webSearch';

definePageMeta({ bleed: true });

const { t } = useI18n();
const route = useRoute();
const orgs = useOrgsStore();
const projects = useProjectsStore();
const agents = useAgentsStore();
const providers = useProvidersStore();
const servers = useMcpServersStore();
const knowledge = useKnowledgeStore();
const webSearch = useWebSearchStore();
const runtimes = useAgentRuntimeStore();
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

await Promise.all([
  agents.ensureLoaded(project.value.id),
  useAuthStore().load(),
  providers.ensureLoaded(project.value.id),
  servers.ensureLoaded(project.value.id),
  knowledge.ensureLoaded(project.value.id),
  webSearch.ensureLoaded(project.value.id),
]);

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
const agentId = agent.value.id;
runtimes.draftFor(agentId, agent.value.slug);
const runtime = computed({
  get: () => runtimes.drafts[agentId]!,
  set: (next) => {
    runtimes.drafts[agentId] = next;
  },
});
const base = computed(() => `/o/${route.params.org}/projects/${route.params.project}`);
const links = computed(() => ({ providers: `${base.value}/settings/model-providers`, servers: `${base.value}/settings/mcp-servers`, webSearch: `${base.value}/settings/web-search`, knowledge: `${base.value}/knowledge` }));
const legacy = computed(() => current.value > 0);
const blockers = computed(() => publishBlockers(runtime.value, { providers: providers.items, servers: servers.items, maxCostCents: config.value.maxCostCents, oldTools: config.value.tools.length }));
const allChanges = computed(() => [...diff.value, ...runtimeChanges(runtime.value, undefined, providers.items)]);
const usesSamples = computed(() => runtimes.usesSample(agentId));
const loaded = computed(() => !!state.value && !draft.loadError.value);
// TODO: allow publishing once the runtime config RPCs land; the preview keeps it off.
const canPublish = computed(() => false);
const blockersId = useId();

function openFixes() {
  publishOpen.value = true;
  void nextTick(() => document.getElementById(blockersId)?.focus());
}

function jump(target: string) {
  publishOpen.value = false;
  void nextTick(() => {
    const section = document.getElementById(`configure-${target}`);
    section?.scrollIntoView({ block: 'start' });
    (section?.querySelector<HTMLElement>(`[data-field="${target}"]`) ?? section?.querySelector<HTMLElement>('select, input, textarea, button'))?.focus({ preventScroll: true });
  });
}

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
const liveLine = computed(() => (current.value ? t('legacy.live_needs_republish', { n: current.value }) : t('configure.never_published')));

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
        <div class="flex flex-wrap items-center gap-2">
          <h1 class="truncate text-xl font-semibold">
            {{ agent.name }}
          </h1>
          <span class="badge badge-info badge-soft text-base-content badge-sm">{{ t('preview.badge') }}</span>
        </div>
        <p :id="reasonId" class="flex flex-wrap gap-x-1 text-ui text-base-content/70">
          <span role="status">{{ draft.loadError.value ? '' : status }}</span>
          <span v-if="!draft.loadError.value">{{ liveLine }}</span>
          <span v-if="canEdit && loaded">{{ usesSamples ? t('configure.samples_block') : t('configure.preview_off') }}</span>
        </p>
      </div>
      <div v-if="canEdit" class="flex items-center gap-2">
        <AppButton variant="plain" size="sm" :disabled="!loaded" @click="publishOpen = true">
          <Icon name="lucide:git-compare" size="16" aria-hidden="true" />
          {{ t('configure.view_changes') }}
        </AppButton>
        <AppButton v-if="loaded && !current && blockers.length" variant="plain" size="sm" @click="openFixes()">
          <Icon name="lucide:circle-alert" size="16" class="text-error" aria-hidden="true" />
          {{ t('publish.fix_count', { n: blockers.length }, blockers.length) }}
        </AppButton>
        <AppButton size="sm" :disabled="!canPublish" :aria-describedby="reasonId" @click="publishOpen = true">
          <Icon name="lucide:upload" size="16" aria-hidden="true" />
          {{ t('configure.publish') }}
        </AppButton>
      </div>
    </div>

    <div class="flex flex-col gap-2 px-4 lg:px-6" :class="{ 'pt-4': legacy && loaded }">
      <LoadError :error="draft.loadError.value" :busy="false" :retry-label="t('common.retry')" @retry="draft.load()" />
      <ErrorText :error="draft.saveError.value" />
      <div v-if="legacy && loaded" role="alert" class="alert alert-warning alert-soft alert-vertical text-base-content text-ui sm:alert-horizontal">
        <Icon name="lucide:triangle-alert" size="16" class="shrink-0" aria-hidden="true" />
        <span>{{ t('legacy.banner', { n: current }) }}</span>
        <AppButton v-if="canEdit" variant="plain" size="xs" @click="jump('model')">
          {{ t('legacy.pick_model') }}
        </AppButton>
      </div>
    </div>

    <div class="grid min-w-0 grid-cols-[minmax(0,1fr)] xl:grid-cols-[minmax(0,1fr)_minmax(22rem,26rem)]">
      <AgentConfigForm
        v-model="config" v-model:runtime="runtime" :providers="providers.items" :servers="servers.items" :sources="knowledge.items"
        :search-ready="!!webSearch.setting.provider" :links="links" :legacy-model="legacy ? (draft.live.value?.model ?? '') : ''" :disabled="!canEdit || !loaded"
      />
      <ConfigurePane :agent-id="agent.id" :config="config" />
    </div>

    <AppModal :open="publishOpen" :title="t('configure.publish_title', { name: agent.name, n: current + 1 })" :dismissible="!draft.publishing.value" wide @close="publishOpen = false">
      <div v-if="blockers.length" :id="blockersId" role="alert" tabindex="-1" class="alert alert-error alert-soft mb-3 items-start text-start text-base-content text-ui">
        <div class="flex w-full flex-col gap-1">
          <p class="flex items-center gap-1.5 font-semibold">
            <Icon name="lucide:circle-alert" size="16" class="shrink-0" aria-hidden="true" />
            {{ t('publish.fix_first') }}
          </p>
          <ul class="flex flex-col gap-1">
            <li v-for="b in blockers" :key="b.key + (b.params?.server ?? '')">
              <button type="button" class="link link-hover text-start" @click="jump(b.target)">
                {{ t(b.key, b.params ?? {}) }}
              </button>
            </li>
          </ul>
        </div>
      </div>
      <p class="text-ui text-base-content/70">
        {{ current ? t('configure.publish_body_legacy', { n: current + 1 }) : t('configure.publish_first') }}
      </p>
      <ul v-if="allChanges.length" class="flex flex-col gap-2 py-3 text-ui">
        <li v-for="(change, i) in allChanges" :key="i" class="flex gap-2">
          <Icon :name="changeIcon(change.key)" size="16" class="mt-0.5 shrink-0" aria-hidden="true" />
          <span>{{ t(change.key, change.params ?? {}) }}</span>
        </li>
      </ul>
      <p v-else class="py-3 text-ui text-base-content/70">
        {{ t('configure.no_changes') }}
      </p>
      <p class="flex items-center gap-1.5 text-xs text-base-content/70">
        <Icon name="lucide:flask-conical" size="14" class="shrink-0" aria-hidden="true" />
        {{ usesSamples ? t('configure.samples_block') : t('configure.preview_off') }}
      </p>
      <ErrorText :error="draft.publishError.value" />
      <template #footer>
        <AppButton variant="plain" size="sm" :disabled="draft.publishing.value" @click="publishOpen = false">
          {{ t('configure.keep_editing') }}
        </AppButton>
        <AppButton size="sm" :busy="draft.publishing.value" :aria-busy="draft.publishing.value || undefined" :disabled="!canPublish" :aria-describedby="blockers.length ? blockersId : reasonId" @click="publish()">
          {{ draft.publishing.value ? t('configure.publishing') : t('configure.publish_n', { n: current + 1 }) }}
        </AppButton>
      </template>
    </AppModal>
  </div>
</template>
