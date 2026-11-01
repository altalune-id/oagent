<script setup lang="ts">
import type { Column } from '#base/components/ui/types';
import type { Agent } from '~/stores/agents';
import ConfirmModal from '#base/components/ui/ConfirmModal.vue';
import CopyButton from '#base/components/ui/CopyButton.vue';
import DataTable from '#base/components/ui/DataTable.vue';
import EmptyState from '#base/components/ui/EmptyState.vue';
import ErrorText from '#base/components/ui/ErrorText.vue';
import PageHeader from '#base/components/ui/PageHeader.vue';
import { useConfirm } from '#base/composables/useConfirm';
import { useDates } from '#base/composables/useDates';
import AgentCreateModal from '~/components/agent/create/AgentCreateModal.vue';
import AgentEditModal from '~/components/agent/edit/AgentEditModal.vue';
import { useCan } from '~/composables/useCan';
import { useUnits } from '~/composables/useUnits';
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
const { when } = useDates();
const { money, seconds, count } = useUnits();

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

const columns = computed<Column[]>(() => [
  { key: 'name', label: t('agent.col_agent') },
  { key: 'id', label: t('common.id') },
  { key: 'actions', label: t('agent.col_actions') },
]);
const sampleColumns = computed<Column[]>(() => [
  { key: 'name', label: t('agent.col_agent') },
  { key: 'version', label: t('agent.col_version') },
  { key: 'model', label: t('agent.col_model') },
  { key: 'runs7d', label: t('agent.col_runs'), align: 'end' },
  { key: 'medianSeconds', label: t('agent.col_median'), align: 'end' },
  { key: 'costCents7d', label: t('agent.col_cost'), align: 'end' },
  { key: 'updatedAt', label: t('agent.col_updated') },
]);

const editModal = ref<InstanceType<typeof AgentEditModal>>();
function edit(agent: Agent) {
  editModal.value?.show(agent);
}

const deleting = useConfirm<Agent>(async (agent) => {
  await client.deleteAgent({ id: agent.id });
  agents.remove(agent.id);
}, 'error.delete_agent');

useHead({ title: () => `${project.value?.name ?? t('project.title')} · ${t('nav.agents')} · OAgent` });
</script>

<template>
  <div v-if="project" class="space-y-6">
    <PageHeader :title="t('nav.agents')" :description="t('agent.subtitle', { project: project.name })" :preview="showSamples ? t('preview.badge') : ''">
      <template #actions>
        <button v-if="showSamples" type="button" class="btn btn-ghost btn-sm" @click="showSamples = false">
          {{ t('agent.hide_samples') }}
        </button>
        <AgentCreateModal v-if="canEdit && (items.length || showSamples)" :org-id="org?.id ?? ''" :project-id="project.id" />
      </template>
    </PageHeader>

    <ErrorText v-if="agents.error" :error="agents.error" />

    <template v-else-if="showSamples">
      <div role="note" class="alert alert-info alert-soft">
        <Icon name="lucide:flask-conical" size="18" />
        <span>{{ t('agent.samples_note') }}</span>
      </div>
      <DataTable :columns="sampleColumns" :rows="samples" row-key="slug" :caption="t('agent.samples_caption')">
        <template #cell-name="{ row }">
          <span class="font-medium">{{ row.name }}</span>
          <span class="block text-xs text-base-content/70">{{ row.description }}</span>
        </template>
        <template #cell-version="{ row }">
          <span class="inline-flex items-center gap-2 whitespace-nowrap">
            v{{ row.version }}
            <span class="badge badge-soft badge-sm" :class="row.draft ? 'badge-warning' : 'badge-success'">{{ row.draft ? t('agent.draft_changes') : t('agent.live') }}</span>
          </span>
        </template>
        <template #cell-model="{ row }">
          <span class="font-mono text-xs">{{ row.model }}</span>
        </template>
        <template #cell-runs7d="{ row }">
          {{ count(row.runs7d) }}
        </template>
        <template #cell-medianSeconds="{ row }">
          {{ seconds(row.medianSeconds) }}
        </template>
        <template #cell-costCents7d="{ row }">
          {{ money(row.costCents7d) }}
        </template>
        <template #cell-updatedAt="{ row }">
          <time :datetime="row.updatedAt.toISOString()" class="whitespace-nowrap">{{ when(row.updatedAt) }}</time>
        </template>
        <template #row-sm="{ row }">
          <div class="list-col-grow">
            <p class="font-medium">
              {{ row.name }}
            </p>
            <p class="text-xs text-base-content/70">
              v{{ row.version }} · {{ row.model }} · {{ count(row.runs7d) }} · {{ money(row.costCents7d) }}
            </p>
          </div>
        </template>
      </DataTable>
    </template>

    <DataTable v-else :columns="columns" :rows="items" row-key="id" :caption="t('nav.agents')">
      <template #cell-name="{ row }">
        <NuxtLink :to="`${base}/${row.slug}`" class="link link-hover font-medium">
          {{ row.name }}
        </NuxtLink>
        <span v-if="row.description" class="block text-xs text-base-content/70">{{ row.description }}</span>
      </template>
      <template #cell-id="{ row }">
        <span class="inline-flex items-center gap-1">
          <span class="font-mono text-xs text-base-content/70">{{ row.id }}</span>
          <CopyButton :value="row.id" :label="t('common.copy')" />
        </span>
      </template>
      <template #cell-actions="{ row }">
        <span class="flex justify-end gap-1 whitespace-nowrap">
          <button v-if="canEdit" type="button" class="btn btn-ghost btn-xs" @click="edit(row)">
            <Icon name="lucide:pencil" size="14" />
            {{ t('common.edit') }}
          </button>
          <button v-if="canEdit" type="button" class="btn btn-ghost btn-xs" @click="deleting.ask(row)">
            <Icon name="lucide:trash-2" size="14" class="text-error" />
            {{ t('common.delete') }}
          </button>
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
        <span class="flex gap-1">
          <button v-if="canEdit" type="button" class="btn btn-ghost btn-xs btn-square" :aria-label="t('common.edit')" @click="edit(row)">
            <Icon name="lucide:pencil" size="14" />
          </button>
          <button v-if="canEdit" type="button" class="btn btn-ghost btn-xs btn-square" :aria-label="t('common.delete')" @click="deleting.ask(row)">
            <Icon name="lucide:trash-2" size="14" class="text-error" />
          </button>
        </span>
      </template>
      <template #empty>
        <EmptyState icon="lucide:bot" :title="t('agent.empty_title', { project: project.name })" :body="t('agent.empty_body')">
          <template #actions>
            <AgentCreateModal v-if="canEdit" :org-id="org?.id ?? ''" :project-id="project.id" />
            <button type="button" class="btn btn-ghost btn-sm" @click="showSamples = true">
              {{ t('agent.show_samples') }}
            </button>
          </template>
        </EmptyState>
      </template>
    </DataTable>

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
