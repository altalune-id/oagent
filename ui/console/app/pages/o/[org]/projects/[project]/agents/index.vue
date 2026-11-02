<script setup lang="ts">
import type { Column, TableSort } from '#base/components/ui/types';
import type { Agent } from '~/stores/agents';
import AppBadge from '#base/components/ui/AppBadge.vue';
import AppButton from '#base/components/ui/AppButton.vue';
import ConfirmModal from '#base/components/ui/ConfirmModal.vue';
import CopyButton from '#base/components/ui/CopyButton.vue';
import DataTable from '#base/components/ui/DataTable.vue';
import EmptyState from '#base/components/ui/EmptyState.vue';
import LoadError from '#base/components/ui/LoadError.vue';
import PageHeader from '#base/components/ui/PageHeader.vue';
import SearchInput from '#base/components/ui/SearchInput.vue';
import { useToast } from '#base/components/ui/toast';
import { useConfirm } from '#base/composables/useConfirm';
import { useDates } from '#base/composables/useDates';
import AgentCreateModal from '~/components/agent/create/AgentCreateModal.vue';
import AgentEditModal from '~/components/agent/edit/AgentEditModal.vue';
import { useCan } from '~/composables/useCan';
import { sampleAgents } from '~/preview/samples';
import { useAgentsStore } from '~/stores/agents';
import { useAuthStore } from '~/stores/auth';
import { useOrgsStore } from '~/stores/orgs';
import { useProjectsStore } from '~/stores/projects';

const route = useRoute();
const { t } = useI18n();
const orgs = useOrgsStore();
const projects = useProjectsStore();
const agents = useAgentsStore();
const { $agentClient: client } = useNuxtApp();
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

const items = computed(() => (project.value ? agents.forProject(project.value.id) : []));
const base = computed(() => `/o/${org.value?.slug}/projects/${project.value?.slug}/agents`);
const showSamples = ref(false);
const samples = sampleAgents();

// NOTE: Recent runs, Runs, Median and Cost need the agent runtime; add their columns here when it reports them.
const columns = computed<Column[]>(() => [
  { key: 'name', label: t('agent.col_agent'), sortable: true },
  { key: 'publishedVersion', label: t('agent.col_version') },
  { key: 'updatedAt', label: t('agent.col_updated'), sortable: true },
  { key: 'id', label: t('common.id') },
  { key: 'actions', label: t('agent.col_actions'), hideLabel: true, align: 'end' },
]);
const sampleColumns = computed<Column[]>(() => [
  { key: 'name', label: t('agent.col_agent'), sortable: true },
  { key: 'version', label: t('agent.col_version') },
  { key: 'model', label: t('agent.col_model'), mono: true },
  { key: 'updatedAt', label: t('agent.col_updated'), sortable: true },
]);

const query = ref('');
const sort = ref<TableSort>({ key: 'updatedAt', dir: 'descending' });

function shown<T extends { name: string, description: string, updatedAt?: Date }>(rows: T[]): T[] {
  const q = query.value.trim().toLowerCase();
  const hits = q ? rows.filter(r => `${r.name} ${r.description}`.toLowerCase().includes(q)) : rows;
  const out = hits.toSorted(sort.value.key === 'name'
    ? (a, b) => a.name.localeCompare(b.name)
    : (a, b) => (a.updatedAt?.getTime() ?? 0) - (b.updatedAt?.getTime() ?? 0));
  return sort.value.dir === 'ascending' ? out : out.toReversed();
}
const rows = computed(() => shown(items.value));
const sampleRows = computed(() => shown(samples));

const editModal = ref<InstanceType<typeof AgentEditModal>>();
function edit(agent: Agent) {
  editModal.value?.show(agent);
}

const deleting = useConfirm<Agent>(async (agent) => {
  await client.deleteAgent({ id: agent.id });
  agents.remove(agent.id);
  toast.push('success', t('agent.deleted', { name: agent.name }));
}, 'error.delete_agent');

useHead({ title: () => `${project.value?.name ?? t('project.title')} · ${t('nav.agents')} · OAgent` });
</script>

<template>
  <div v-if="project" class="space-y-6">
    <PageHeader :title="t('nav.agents')" :description="t('agent.subtitle', { project: project.name })" :preview="showSamples ? t('preview.badge') : ''">
      <template #actions>
        <AppButton v-if="showSamples" variant="ghost" size="sm" @click="showSamples = false">
          {{ t('agent.hide_samples') }}
        </AppButton>
        <AgentCreateModal v-if="canEdit && (items.length || showSamples)" :org-id="org?.id ?? ''" :project-id="project.id" />
      </template>
    </PageHeader>

    <LoadError v-if="agents.error" :error="agents.error" :busy="agents.loading" :retry-label="t('common.retry')" @retry="agents.load(project.id)" />

    <template v-else>
      <div v-if="showSamples || items.length" class="flex flex-wrap items-center justify-between gap-2">
        <SearchInput v-model="query" :label="t('agent.filter')" />
      </div>

      <template v-if="showSamples">
        <div role="note" class="alert alert-info alert-soft">
          <Icon name="lucide:flask-conical" size="18" />
          <span>{{ t('agent.samples_note') }}</span>
        </div>
        <DataTable v-model:sort="sort" :columns="sampleColumns" :rows="sampleRows" row-key="slug" :caption="t('agent.samples_caption')">
          <template #cell-name="{ row }">
            <span class="inline-flex items-center gap-2 font-medium whitespace-nowrap">
              {{ row.name }}
              <AppBadge :label="t('agent.sample')" />
            </span>
          </template>
          <template #cell-version="{ row }">
            <span class="whitespace-nowrap"><span class="font-mono text-xs">v{{ row.version }}</span> <AppBadge :label="row.draft ? t('agent.draft_changes') : t('agent.live')" :outline="row.draft" /></span>
          </template>
          <template #cell-updatedAt="{ row }">
            <time :datetime="row.updatedAt.toISOString()" class="whitespace-nowrap text-base-content/70">{{ when(row.updatedAt) }}</time>
          </template>
          <template #row-sm="{ row }">
            <div class="list-col-grow">
              <p class="flex flex-wrap items-center gap-2 font-medium">
                {{ row.name }}
                <AppBadge :label="t('agent.sample')" />
              </p>
              <p class="text-xs text-base-content/70">
                <span class="font-mono">v{{ row.version }}</span> · <span class="font-mono">{{ row.model }}</span>
              </p>
            </div>
          </template>
          <template #empty>
            <p class="text-ui text-base-content/70">
              {{ t('agent.no_match', { query }) }}
            </p>
          </template>
        </DataTable>
      </template>

      <DataTable v-else v-model:sort="sort" :columns="columns" :rows="rows" row-key="id" :caption="t('nav.agents')">
        <template #cell-name="{ row }">
          <NuxtLink :to="`${base}/${row.slug}`" class="link link-hover font-medium whitespace-nowrap">
            {{ row.name }}
          </NuxtLink>
        </template>
        <template #cell-publishedVersion="{ row }">
          <span class="whitespace-nowrap">
            <template v-if="row.publishedVersion"><span class="font-mono text-xs">v{{ row.publishedVersion }}</span> </template>
            <AppBadge v-if="!row.publishedVersion" :label="t('agent.not_published')" outline />
            <AppBadge v-else-if="row.unpublishedChanges" :label="t('agent.draft_changes')" outline />
            <AppBadge v-else :label="t('agent.live')" />
          </span>
        </template>
        <template #cell-updatedAt="{ row }">
          <time v-if="row.updatedAt" :datetime="row.updatedAt.toISOString()" class="whitespace-nowrap text-base-content/70">{{ when(row.updatedAt) }}</time>
        </template>
        <template #cell-id="{ row }">
          <span class="inline-flex items-center gap-1">
            <span class="font-mono text-xs text-base-content/70">{{ row.id }}</span>
            <CopyButton :value="row.id" :label="t('common.copy')" />
          </span>
        </template>
        <template #cell-actions="{ row }">
          <span v-if="canEdit" class="flex justify-end gap-1 whitespace-nowrap">
            <AppButton variant="ghost" size="xs" square :aria-label="t('agent.edit_named', { name: row.name })" @click="edit(row)">
              <Icon name="lucide:pencil" size="14" aria-hidden="true" />
            </AppButton>
            <AppButton variant="ghost" size="xs" square :aria-label="t('agent.delete_named', { name: row.name })" @click="deleting.ask(row)">
              <Icon name="lucide:trash-2" size="14" class="text-error" aria-hidden="true" />
            </AppButton>
          </span>
        </template>
        <template #row-sm="{ row }">
          <div class="list-col-grow min-w-0">
            <NuxtLink :to="`${base}/${row.slug}`" class="link link-hover font-medium">
              {{ row.name }}
            </NuxtLink>
            <p v-if="row.description" class="text-xs text-base-content/70">
              {{ row.description }}
            </p>
          </div>
          <span v-if="canEdit" class="flex gap-1">
            <AppButton variant="ghost" size="xs" square :aria-label="t('agent.edit_named', { name: row.name })" @click="edit(row)">
              <Icon name="lucide:pencil" size="14" aria-hidden="true" />
            </AppButton>
            <AppButton variant="ghost" size="xs" square :aria-label="t('agent.delete_named', { name: row.name })" @click="deleting.ask(row)">
              <Icon name="lucide:trash-2" size="14" class="text-error" aria-hidden="true" />
            </AppButton>
          </span>
        </template>
        <template #empty>
          <p v-if="items.length" class="text-ui text-base-content/70">
            {{ t('agent.no_match', { query }) }}
          </p>
          <EmptyState v-else icon="lucide:bot" :title="t('agent.empty_title', { project: project.name })" :body="t('agent.empty_body')">
            <template #actions>
              <AgentCreateModal v-if="canEdit" :org-id="org?.id ?? ''" :project-id="project.id" />
              <AppButton variant="plain" size="sm" @click="showSamples = true">
                {{ t('agent.show_samples') }}
              </AppButton>
            </template>
          </EmptyState>
        </template>
      </DataTable>
    </template>

    <AgentEditModal v-if="canEdit" ref="editModal" />
    <ConfirmModal
      v-if="canEdit"
      :handle="deleting"
      :title="t('agent.delete_title')"
      :body="t('agent.delete_body', { name: deleting.target.value?.name ?? '' })"
      :confirm-label="t('common.delete')"
      :cancel-label="t('common.cancel')"
    />
  </div>
</template>
