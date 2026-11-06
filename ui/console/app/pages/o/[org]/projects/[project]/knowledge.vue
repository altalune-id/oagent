<script setup lang="ts">
import type { Column } from '#base/components/ui/types';
import type { KnowledgeSource } from '~/stores/knowledge';
import AppButton from '#base/components/ui/AppButton.vue';
import AppDropdown from '#base/components/ui/AppDropdown.vue';
import ConfirmModal from '#base/components/ui/ConfirmModal.vue';
import DataTable from '#base/components/ui/DataTable.vue';
import EmptyState from '#base/components/ui/EmptyState.vue';
import PageHeader from '#base/components/ui/PageHeader.vue';
import StatusLabel from '#base/components/ui/StatusLabel.vue';
import { useToast } from '#base/components/ui/toast';
import { useConfirm } from '#base/composables/useConfirm';
import { useDates } from '#base/composables/useDates';
import AddSourceModal from '~/components/knowledge/AddSourceModal.vue';
import { knowledgeTone } from '~/components/knowledge/knowledge';
import PreviewNote from '~/components/layout/PreviewNote.vue';
import ProjectReadOnlyNote from '~/components/project/ProjectReadOnlyNote.vue';
import { useProject } from '~/components/project/useProject';
import { useCan } from '~/composables/useCan';
import { useAuthStore } from '~/stores/auth';
import { useKnowledgeStore } from '~/stores/knowledge';

const { t } = useI18n();
const route = useRoute();
const toast = useToast();
const { when } = useDates();
const { project } = useProject();
const canEdit = useCan('project:write');
const store = useKnowledgeStore();
void store.ensureLoaded(project.value!.id);

const canEmbed = computed(() => store.embeddingReady);
const providersLink = computed(() => `/o/${route.params.org}/projects/${route.params.project}/settings/model-providers`);
const adding = ref(false);

const columns = computed<Column[]>(() => [
  { key: 'name', label: t('knowledge.col_name') },
  { key: 'kind', label: t('knowledge.col_type') },
  { key: 'status', label: t('knowledge.col_status') },
  { key: 'size', label: t('knowledge.col_size') },
  { key: 'usedBy', label: t('knowledge.col_used_by') },
  { key: 'updated', label: t('knowledge.col_updated'), muted: true },
  ...(canEdit.value ? [{ key: 'menu', label: t('knowledge.col_actions'), hideLabel: true, align: 'end' as const }] : []),
]);

const deleting = useConfirm<KnowledgeSource>(async (s) => {
  store.remove(s.id);
  toast.push('success', t('knowledge.deleted', { name: s.name }));
}, 'error.delete_knowledge');

function retry(s: KnowledgeSource) {
  store.retry(s.id);
  toast.push('success', t('knowledge.retried', { name: s.name }));
}

function add(url: string, maxPages: number) {
  const source = store.addWebsite(url, maxPages);
  adding.value = false;
  toast.push('success', t('knowledge.added', { name: source.name }));
}

useHead({ title: () => `${t('knowledge.title')} · ${project.value?.name ?? ''} · OAgent` });

await useAuthStore().load();
</script>

<template>
  <div class="flex flex-col gap-4">
    <PageHeader :title="t('knowledge.title')" :description="t('knowledge.subtitle')" :preview="t('preview.badge')">
      <template #status>
        <PreviewNote :text="t('preview.local_note')" />
      </template>
      <template v-if="canEdit && store.items.length" #actions>
        <AppButton size="sm" :disabled="!canEmbed" @click="adding = true">
          <Icon name="lucide:plus" size="16" aria-hidden="true" />
          {{ t('knowledge.add') }}
        </AppButton>
      </template>
    </PageHeader>
    <ProjectReadOnlyNote v-if="!canEdit" />
    <div v-else-if="!canEmbed" role="note" class="alert items-start text-ui">
      <Icon name="lucide:info" size="16" class="shrink-0 text-warning" aria-hidden="true" />
      <span>
        {{ t('knowledge.needs_embedding') }}
        <NuxtLink :to="providersLink" class="link">{{ t('knowledge.open_providers') }}</NuxtLink>
      </span>
    </div>

    <DataTable stacked :caption="t('knowledge.title')" row-key="id" :columns="columns" :rows="store.items" :loading="store.loading">
      <template #empty>
        <EmptyState icon="lucide:library" :title="t('knowledge.empty')" :body="t('knowledge.empty_body')">
          <template v-if="canEdit" #actions>
            <AppButton size="sm" :disabled="!canEmbed" @click="adding = true">
              <Icon name="lucide:plus" size="16" aria-hidden="true" />
              {{ t('knowledge.add') }}
            </AppButton>
          </template>
        </EmptyState>
      </template>
      <template #cell-name="{ row }">
        <span class="flex min-w-0 flex-col">
          <span class="font-medium">{{ row.name }}</span>
          <span v-if="row.url" class="block max-w-64 truncate font-mono text-xs text-base-content/70" :title="row.url">{{ row.url }}</span>
        </span>
      </template>
      <template #cell-kind="{ row }">
        {{ t(`knowledge.kind_${row.kind}`) }}
      </template>
      <template #cell-status="{ row }">
        <span class="flex flex-col gap-1">
          <StatusLabel :tone="knowledgeTone[(row as KnowledgeSource).status]" :label="t(`knowledge.status_${row.status}`)" />
          <template v-if="row.kind === 'website' && (row.status === 'crawling' || row.status === 'queued')">
            <progress class="progress progress-info w-32" :value="row.pagesDone ?? 0" :max="row.pagesTotal ?? 1" aria-hidden="true" />
            <span class="text-xs text-base-content/70 tabular-nums">{{ t('knowledge.pages_progress', { done: row.pagesDone ?? 0, total: row.pagesTotal ?? 0 }) }}</span>
          </template>
          <span v-else-if="row.failure" class="text-xs text-base-content/70">{{ t(`knowledge.failure_${row.failure.reason}`, { path: row.failure.path ?? '' }) }}</span>
        </span>
      </template>
      <template #cell-size="{ row }">
        <span class="tabular-nums">{{ row.chunks ? t('knowledge.chunks', { n: row.chunks }, row.chunks) : '—' }}</span>
      </template>
      <template #cell-usedBy="{ row }">
        <span v-if="row.usedBy.length">{{ row.usedBy.join(', ') }}</span>
        <span v-else class="text-base-content/70">{{ t('knowledge.unused') }}</span>
      </template>
      <template #cell-updated="{ row }">
        {{ when(row.updatedAt) }}
      </template>
      <template #cell-menu="{ row }">
        <AppDropdown placement="end" size="xs" trigger-class="btn-square" content-class="w-44" :label="t('knowledge.actions_for', { name: row.name })">
          <template #trigger>
            <Icon name="lucide:ellipsis" size="14" class="shrink-0" aria-hidden="true" />
          </template>
          <ul class="menu menu-sm w-full p-0 text-ui">
            <li v-if="row.status === 'failed'">
              <button type="button" @click="retry(row as KnowledgeSource)">
                <Icon name="lucide:rotate-cw" size="16" class="shrink-0" aria-hidden="true" />
                {{ t('knowledge.retry') }}
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

    <AddSourceModal :open="adding" @close="adding = false" @save="add" />
    <ConfirmModal
      :handle="deleting"
      :title="t('knowledge.delete_title')"
      :body="t('knowledge.delete_body', { name: deleting.target.value?.name ?? '' })"
      :confirm-label="t('common.delete')"
      :cancel-label="t('common.cancel')"
    />
  </div>
</template>
