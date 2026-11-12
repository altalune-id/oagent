<script setup lang="ts">
import type { Column } from '#base/components/ui/types';
import type { ShownError } from '#base/lib/apperror/message';
import type { KnowledgeSource } from '~/stores/knowledge';
import AppButton from '#base/components/ui/AppButton.vue';
import AppDropdown from '#base/components/ui/AppDropdown.vue';
import { confirmHandle } from '#base/components/ui/confirmHandle';
import ConfirmModal from '#base/components/ui/ConfirmModal.vue';
import DataTable from '#base/components/ui/DataTable.vue';
import EmptyState from '#base/components/ui/EmptyState.vue';
import LoadError from '#base/components/ui/LoadError.vue';
import PageHeader from '#base/components/ui/PageHeader.vue';
import StatusLabel from '#base/components/ui/StatusLabel.vue';
import { useToast } from '#base/components/ui/toast';
import { useDates } from '#base/composables/useDates';
import { useErrorMessage } from '#base/composables/useErrorMessage';
import { errorCode } from '#base/lib/apperror/message';
import AddSourceModal from '~/components/knowledge/AddSourceModal.vue';
import EmbeddingCard from '~/components/knowledge/EmbeddingCard.vue';
import { failureText, sourceState } from '~/components/knowledge/knowledge';
import { useKnowledgePolling } from '~/components/knowledge/useKnowledgePolling';
import { useSourceCreate } from '~/components/knowledge/useSourceCreate';
import ProjectReadOnlyNote from '~/components/project/ProjectReadOnlyNote.vue';
import { useProject } from '~/components/project/useProject';
import { useCan } from '~/composables/useCan';
import { useAgentsStore } from '~/stores/agents';
import { useAuthStore } from '~/stores/auth';
import { useKnowledgeStore } from '~/stores/knowledge';
import { useModelProvidersStore } from '~/stores/modelProviders';

const { t, te } = useI18n();
const route = useRoute();
const toast = useToast();
const { when } = useDates();
const errorMessage = useErrorMessage();
const { org, project } = useProject();
const canEdit = useCan('knowledge:write');
const store = useKnowledgeStore();
const providers = useModelProvidersStore();
const agents = useAgentsStore();
const projectId = project.value!.id;
const base = computed(() => `/o/${route.params.org}/projects/${route.params.project}`);
const form = useSourceCreate(org.value!.id, projectId);
const polling = useKnowledgePolling(projectId);

const columns = computed<Column[]>(() => [
  { key: 'name', label: t('knowledge.col_name') },
  { key: 'kind', label: t('knowledge.col_type') },
  { key: 'status', label: t('knowledge.col_status') },
  { key: 'documents', label: t('knowledge.col_documents'), align: 'end' },
  { key: 'usedBy', label: t('knowledge.col_used_by') },
  { key: 'updated', label: t('knowledge.col_updated'), muted: true },
  ...(canEdit.value ? [{ key: 'menu', label: t('knowledge.col_actions'), hideLabel: true, align: 'end' as const }] : []),
]);

const usedByNames = (s: KnowledgeSource) => s.usedBy.map(a => a.name).join(', ');
function agentLink(id: string) {
  const slug = agents.items.find(a => a.id === id)?.slug;
  return slug ? `${base.value}/agents/${slug}` : '';
}
const deleting = confirmHandle<KnowledgeSource>(async (s) => {
  await store.removeSource(s.id);
  toast.push('success', t('knowledge.deleted', { name: s.name }));
}, deleteError);

function deleteError(e: unknown): ShownError {
  return errorCode(e) === 'KNW008' ? { message: t('knowledge.in_use', { agents: usedByNames(deleting.target.value!) }) } : errorMessage(e, 'error.delete_knowledge');
}

async function crawl(s: KnowledgeSource) {
  try {
    await store.refresh(s.id);
    toast.push('success', t('knowledge.crawling_again', { name: s.name }));
    polling.start();
  } catch (e) {
    toast.push('error', errorMessage(e, 'error.refresh_knowledge').message);
  }
}

async function created(s: KnowledgeSource) {
  toast.push('success', t('knowledge.added', { name: s.name }));
  await navigateTo(`${base.value}/knowledge/${s.id}`);
}

const failure = (s: KnowledgeSource) => (s.failure ? failureText(s.failure, t, te) : '');

useHead({ title: () => `${t('knowledge.title')} · ${project.value?.name ?? ''} · OAgent` });

await Promise.all([useAuthStore().load(), store.load(projectId), providers.ensureLoaded(projectId), agents.ensureLoaded(projectId)]);
</script>

<template>
  <div class="flex flex-col gap-4">
    <PageHeader :title="t('knowledge.title')" :description="t('knowledge.subtitle')">
      <template v-if="canEdit && store.on && store.sources.length" #actions>
        <AppButton size="sm" @click="form.show()">
          <Icon name="lucide:plus" size="16" aria-hidden="true" />
          {{ t('knowledge.add') }}
        </AppButton>
      </template>
    </PageHeader>
    <ProjectReadOnlyNote v-if="!canEdit" :text="t('knowledge.read_only')" />

    <LoadError v-if="store.error" :error="store.error" :busy="store.loading" :retry-label="t('common.retry')" @retry="store.load(projectId)" />
    <EmptyState v-else-if="!store.loading && !store.on" icon="lucide:book-open" :title="t('knowledge.off')" :body="t('knowledge.off_body')" />
    <template v-else>
      <EmbeddingCard :project-id="projectId" :providers="providers.items" :can-edit="canEdit" :providers-link="`${base}/settings/model-providers`" @saved="polling.start()" />

      <DataTable stacked :caption="t('knowledge.sources')" row-key="id" :columns="columns" :rows="store.sources" :loading="store.loading">
        <template #empty>
          <EmptyState icon="lucide:book-open" :title="t('knowledge.empty')" :body="t('knowledge.empty_body')">
            <template v-if="canEdit" #actions>
              <AppButton size="sm" @click="form.show()">
                <Icon name="lucide:plus" size="16" aria-hidden="true" />
                {{ t('knowledge.add') }}
              </AppButton>
            </template>
          </EmptyState>
        </template>
        <template #cell-name="{ row }">
          <span class="flex min-w-0 flex-col">
            <NuxtLink :to="`${base}/knowledge/${row.id}`" class="link link-hover font-medium">{{ row.name }}</NuxtLink>
            <span v-if="row.seedUrl" class="block max-w-64 truncate font-mono text-xs text-base-content/70" :title="row.seedUrl">{{ row.seedUrl }}</span>
          </span>
        </template>
        <template #cell-kind="{ row }">
          {{ t(`knowledge.kind_${row.kind}`) }}
        </template>
        <template #cell-status="{ row }">
          <span class="flex flex-col gap-1">
            <StatusLabel :tone="sourceState(row as KnowledgeSource).tone" :label="t(sourceState(row as KnowledgeSource).key, sourceState(row as KnowledgeSource).params ?? {})" />
            <span v-if="failure(row as KnowledgeSource)" class="text-xs text-base-content/70">{{ failure(row as KnowledgeSource) }}</span>
          </span>
        </template>
        <template #cell-documents="{ row }">
          <span class="tabular-nums">{{ row.documents ? t('knowledge.documents_ready', { ready: row.ready, n: row.documents }, row.documents) : '—' }}</span>
        </template>
        <template #cell-usedBy="{ row }">
          <span v-if="row.usedBy.length">{{ usedByNames(row as KnowledgeSource) }}</span>
          <span v-else class="text-base-content/70">{{ t('knowledge.unused') }}</span>
        </template>
        <template #cell-updated="{ row }">
          {{ row.updatedAt ? when(row.updatedAt) : '—' }}
        </template>
        <template #cell-menu="{ row }">
          <AppDropdown placement="end" size="xs" trigger-class="btn-square" content-class="w-48" :label="t('knowledge.actions_for', { name: row.name })">
            <template #trigger>
              <Icon name="lucide:ellipsis" size="14" class="shrink-0" aria-hidden="true" />
            </template>
            <ul class="menu menu-sm w-full p-0 text-ui">
              <li v-if="row.kind === 'url'">
                <button type="button" :disabled="row.status === 'crawling'" @click="crawl(row as KnowledgeSource)">
                  <Icon name="lucide:refresh-cw" size="16" class="shrink-0" aria-hidden="true" />
                  {{ t('knowledge.crawl_again') }}
                </button>
              </li>
              <li>
                <button type="button" @click="deleting.ask(row as KnowledgeSource)">
                  <Icon name="lucide:trash-2" size="16" class="shrink-0 text-error" aria-hidden="true" />
                  {{ t('common.delete') }}
                </button>
              </li>
            </ul>
          </AppDropdown>
        </template>
      </DataTable>
    </template>

    <AddSourceModal :form="form" :storage-configured="store.storageConfigured" @created="created" />
    <ConfirmModal
      :handle="deleting"
      :title="deleting.target.value?.usedBy.length ? t('knowledge.in_use_title', { name: deleting.target.value.name }) : t('knowledge.delete_title')"
      :body="deleting.target.value?.usedBy.length ? t('knowledge.in_use_body') : t('knowledge.delete_body', { name: deleting.target.value?.name ?? '' })"
      :confirm-label="t('common.delete')"
      :cancel-label="t('common.cancel')"
      :confirm-disabled="!!deleting.target.value?.usedBy.length"
    >
      <ul v-if="deleting.target.value?.usedBy.length" class="mt-2 flex flex-col gap-1 text-ui">
        <li v-for="a in deleting.target.value.usedBy" :key="a.id">
          <NuxtLink v-if="agentLink(a.id)" :to="agentLink(a.id)" class="link">
            {{ a.name }}
          </NuxtLink>
          <span v-else>{{ a.name }}</span>
        </li>
      </ul>
    </ConfirmModal>
  </div>
</template>
