<script setup lang="ts">
import type { Column } from '#base/components/ui/types';
import type { UploadItem, UploadReject } from '#base/components/ui/upload';
import type { ShownError } from '#base/lib/apperror/message';
import type { KnowledgeDocument, KnowledgeHit } from '~/stores/knowledge';
import AppButton from '#base/components/ui/AppButton.vue';
import { confirmHandle } from '#base/components/ui/confirmHandle';
import ConfirmModal from '#base/components/ui/ConfirmModal.vue';
import DataTable from '#base/components/ui/DataTable.vue';
import EmptyState from '#base/components/ui/EmptyState.vue';
import ErrorText from '#base/components/ui/ErrorText.vue';
import FileUpload from '#base/components/ui/FileUpload.vue';
import LoadError from '#base/components/ui/LoadError.vue';
import PageHeader from '#base/components/ui/PageHeader.vue';
import Section from '#base/components/ui/Section.vue';
import StatusLabel from '#base/components/ui/StatusLabel.vue';
import { useToast } from '#base/components/ui/toast';
import { useDates } from '#base/composables/useDates';
import { useErrorMessage } from '#base/composables/useErrorMessage';
import { formatBytes } from '#base/lib/format/bytes';
import { documentState, failureText, sourceState } from '~/components/knowledge/knowledge';
import { useKnowledgePolling } from '~/components/knowledge/useKnowledgePolling';
import ProjectReadOnlyNote from '~/components/project/ProjectReadOnlyNote.vue';
import { useProject } from '~/components/project/useProject';
import { useCan } from '~/composables/useCan';
import { useAuthStore } from '~/stores/auth';
import { useKnowledgeStore } from '~/stores/knowledge';

const { t, te, locale } = useI18n();
const route = useRoute();
const toast = useToast();
const { when } = useDates();
const errorMessage = useErrorMessage();
const { project } = useProject();
const canEdit = useCan('knowledge:write');
const canRead = useCan('knowledge:read');
const store = useKnowledgeStore();
const projectId = project.value!.id;
const sourceId = route.params.source as string;
const base = `/o/${route.params.org}/projects/${route.params.project}`;
const uid = useId();
const polling = useKnowledgePolling(projectId, sourceId);

await Promise.all([useAuthStore().load(), store.ensureLoaded(projectId)]);
const source = computed(() => store.byId(sourceId));
if (!source.value) {
  throw createError({ statusCode: 404, statusMessage: t('error.knowledge_source_not_found'), fatal: true });
}
await store.loadDocuments(sourceId);

const docs = computed(() => store.documents[sourceId] ?? []);
const crumbs = computed(() => [{ label: t('knowledge.title'), to: `${base}/knowledge` }, { label: source.value?.name ?? '' }]);
const state = computed(() => source.value && sourceState(source.value));
const max = computed(() => formatBytes(store.maxUploadBytes, locale.value));
const uploadItems = computed<UploadItem[]>(() => (store.uploads[sourceId] ?? []).map((u) => {
  const doc = u.state === 'done' ? docs.value.find(d => d.id === u.documentId && d.status === 'failed') : undefined;
  const error = doc?.failure ? failureText(doc.failure, t, te) : u.reason ? t(`knowledge.reject_${u.reason}`) : u.error;
  return { key: u.key, name: u.name, size: formatBytes(u.bytes, locale.value), state: doc ? 'failed' : u.state, progress: u.progress, error };
}));
const uploadStates = computed(() => ({ queued: t('knowledge.upload_queued'), uploading: t('knowledge.upload_uploading'), done: t('knowledge.upload_done'), failed: t('knowledge.upload_failed') }));

const columns = computed<Column[]>(() => [
  { key: 'title', label: t('knowledge.col_document') },
  { key: 'status', label: t('knowledge.col_status') },
  { key: 'pages', label: t('knowledge.col_pages'), align: 'end' },
  ...(docs.value.some(d => d.bytes !== undefined) ? [{ key: 'bytes', label: t('knowledge.col_size'), align: 'end' as const }] : []),
  ...(docs.value.some(d => d.passages !== undefined) ? [{ key: 'passages', label: t('knowledge.col_passages'), align: 'end' as const }] : []),
  { key: 'updated', label: t('knowledge.col_updated'), muted: true },
  ...(canEdit.value && source.value?.kind === 'upload' ? [{ key: 'menu', label: t('knowledge.col_actions'), hideLabel: true, align: 'end' as const }] : []),
]);

const failure = (f?: { code: string, reason: string }) => (f ? failureText(f, t, te) : '');

async function crawl() {
  try {
    await store.refresh(sourceId);
    toast.push('success', t('knowledge.crawling_again', { name: source.value!.name }));
    polling.start();
  } catch (e) {
    toast.push('error', errorMessage(e, 'error.refresh_knowledge').message);
  }
}

async function send(files: File[]) {
  await store.upload(sourceId, files);
  await store.load(projectId, true);
  polling.start();
}

function refuse(rejects: UploadReject[]) {
  store.refuse(sourceId, rejects);
}

const deleting = confirmHandle<KnowledgeDocument>(async (d) => {
  await store.removeDocument(sourceId, d.id);
  toast.push('success', t('knowledge.document_deleted', { name: d.title }));
}, e => errorMessage(e, 'error.delete_knowledge_document'));

const query = ref('');
const searching = ref(false);
const searched = ref(false);
const hits = ref<KnowledgeHit[]>([]);
const searchError = ref<ShownError | null>(null);
const canSearch = computed(() => (source.value?.ready ?? 0) > 0 || docs.value.some(d => d.status === 'ready'));
const hasModel = computed(() => !!store.embedding && !store.embedding.providerMissing);

async function search() {
  if (!query.value.trim() || searching.value) {
    return;
  }
  searching.value = true;
  searchError.value = null;
  try {
    hits.value = await store.search(sourceId, query.value);
    searched.value = true;
  } catch (e) {
    hits.value = [];
    searched.value = false;
    searchError.value = errorMessage(e, 'error.test_search');
  } finally {
    searching.value = false;
  }
}

useHead({ title: () => `${source.value?.name ?? ''} · ${t('knowledge.title')} · OAgent` });
</script>

<template>
  <div v-if="source" class="flex flex-col gap-4">
    <PageHeader :title="source.name" :crumbs="crumbs" :crumbs-label="t('common.breadcrumb')">
      <template #meta>
        <span class="badge badge-ghost badge-sm">{{ t(`knowledge.kind_${source.kind}`) }}</span>
        <StatusLabel v-if="state" :tone="state.tone" :label="t(state.key, state.params ?? {})" />
      </template>
      <template v-if="canEdit && source.kind === 'url'" #actions>
        <AppButton variant="neutral" soft size="sm" :disabled="source.status === 'crawling'" @click="crawl()">
          <Icon name="lucide:refresh-cw" size="16" aria-hidden="true" />
          {{ t('knowledge.crawl_again') }}
        </AppButton>
      </template>
    </PageHeader>
    <ProjectReadOnlyNote v-if="!canEdit" :text="t('knowledge.read_only')" />

    <div v-if="source.failure" role="alert" class="alert alert-soft alert-error items-start text-ui text-base-content">
      <Icon name="lucide:circle-alert" size="16" class="shrink-0" aria-hidden="true" />
      <span>{{ failure(source.failure) }}</span>
    </div>

    <Section v-if="source.kind === 'url'" :title="t('knowledge.website')">
      <dl class="flex flex-col gap-2 text-ui">
        <div class="flex flex-col gap-0.5">
          <dt class="text-xs text-base-content/70">
            {{ t('knowledge.url') }}
          </dt>
          <dd class="font-mono text-xs break-all">
            <a :href="source.seedUrl" target="_blank" rel="noopener noreferrer" class="link">{{ source.seedUrl }}</a>
          </dd>
        </div>
      </dl>
      <p class="mt-2 text-xs text-base-content/70">
        {{ t('knowledge.crawl_limits') }} {{ t('knowledge.crawl_again_hint') }}
      </p>
    </Section>

    <Section v-else-if="canEdit" :title="t('knowledge.upload_title')">
      <template v-if="store.storageConfigured">
        <FileUpload
          :label="t('knowledge.choose_files')"
          :hint="t('knowledge.upload_hint', { max })"
          :drop="t('knowledge.drop_files')"
          :queue="t('knowledge.upload_queue')"
          :states="uploadStates"
          :accept="store.uploadAccept"
          :max-bytes="store.maxUploadBytes"
          :items="uploadItems"
          @select="send"
          @reject="refuse"
        />
        <p class="mt-2 flex items-center gap-1.5 text-xs text-base-content/70">
          <Icon name="lucide:info" size="14" class="shrink-0" aria-hidden="true" />
          {{ t('knowledge.keep_open') }}
        </p>
      </template>
      <p v-else role="note" class="flex items-start gap-1.5 text-ui">
        <Icon name="lucide:hard-drive" size="16" class="mt-0.5 shrink-0 text-warning" aria-hidden="true" />
        {{ t('knowledge.storage_off') }}
      </p>
    </Section>

    <LoadError v-if="store.documentsError" :error="store.documentsError" :busy="store.documentsLoading" :retry-label="t('common.retry')" @retry="store.loadDocuments(sourceId)" />
    <DataTable v-else stacked :caption="source.kind === 'url' ? t('knowledge.pages') : t('knowledge.documents')" row-key="id" :columns="columns" :rows="docs">
      <template #empty>
        <EmptyState
          v-if="source.kind === 'url'" icon="lucide:globe" :heading-level="3"
          :title="t('knowledge.no_pages')" :body="source.status === 'crawling' ? t('knowledge.no_pages_crawling') : t('knowledge.no_pages_body')"
        />
        <EmptyState v-else icon="lucide:files" :heading-level="3" :title="t('knowledge.no_files')" :body="canEdit ? t('knowledge.no_files_body', { max }) : t('knowledge.no_files_readonly')" />
      </template>
      <template #cell-title="{ row }">
        <span class="flex min-w-0 flex-col">
          <span class="font-medium break-words">{{ row.title || row.url }}</span>
          <a v-if="row.url && row.title && row.title !== row.url" :href="row.url" target="_blank" rel="noopener noreferrer" class="link block max-w-72 truncate font-mono text-xs" :title="row.url">{{ row.url }}</a>
        </span>
      </template>
      <template #cell-status="{ row }">
        <span class="flex flex-col gap-1">
          <StatusLabel :tone="documentState(row as KnowledgeDocument, hasModel).tone" :label="t(documentState(row as KnowledgeDocument, hasModel).key)" />
          <span v-if="row.failure || documentState(row as KnowledgeDocument, hasModel).key === 'knowledge.doc_waiting_model'" class="text-xs text-base-content/70">
            {{ row.failure ? failure(row.failure) : '' }}
            <NuxtLink v-if="documentState(row as KnowledgeDocument, hasModel).key === 'knowledge.doc_waiting_model'" :to="`${base}/knowledge`" class="link">{{ t('knowledge.choose_model') }}</NuxtLink>
          </span>
        </span>
      </template>
      <template #cell-pages="{ row }">
        <span class="tabular-nums">{{ row.pages || '—' }}</span>
      </template>
      <template #cell-bytes="{ row }">
        <span class="whitespace-nowrap tabular-nums">{{ row.bytes ? formatBytes(row.bytes, locale) : '—' }}</span>
      </template>
      <template #cell-passages="{ row }">
        <span class="tabular-nums">{{ row.passages ?? '—' }}</span>
      </template>
      <template #cell-updated="{ row }">
        <span class="whitespace-nowrap">{{ row.updatedAt ? when(row.updatedAt) : '—' }}</span>
      </template>
      <template #cell-menu="{ row }">
        <AppButton variant="ghost" size="xs" square class="max-sm:size-11" :aria-label="t('knowledge.delete_document', { name: row.title || row.url })" :title="t('knowledge.delete_document', { name: row.title || row.url })" @click="deleting.ask(row as KnowledgeDocument)">
          <Icon name="lucide:trash-2" size="14" class="text-error" aria-hidden="true" />
        </AppButton>
      </template>
    </DataTable>

    <Section v-if="canRead" :title="t('knowledge.test_search')" :description="t('knowledge.test_search_body')">
      <form novalidate class="flex flex-col gap-2" @submit.prevent="search()">
        <div class="flex flex-col gap-2 sm:join sm:flex-row sm:gap-0">
          <label class="w-full">
            <span class="sr-only">{{ t('knowledge.query') }}</span>
            <input v-model="query" type="search" maxlength="2000" class="input input-sm w-full sm:join-item" :placeholder="t('knowledge.query_placeholder')" :disabled="!canSearch" :aria-describedby="canSearch ? undefined : `${uid}-search-hint`">
          </label>
          <AppButton type="submit" size="sm" class="sm:join-item" :busy="searching" :disabled="!canSearch || !query.trim()">
            <Icon name="lucide:search" size="16" aria-hidden="true" />
            {{ t('knowledge.search') }}
          </AppButton>
        </div>
        <p v-if="!canSearch" :id="`${uid}-search-hint`" class="text-xs text-base-content/70">
          {{ source.kind === 'url' ? t('knowledge.search_needs_ready_page') : t('knowledge.search_needs_ready') }}
        </p>
      </form>
      <ErrorText :error="searchError" />
      <ol v-if="hits.length" class="mt-3 flex flex-col divide-y divide-base-300 rounded-box border border-base-300" :aria-label="t('knowledge.results')">
        <li v-for="(h, i) in hits" :key="i" class="flex flex-col gap-1 p-3 text-ui">
          <div class="flex flex-wrap items-baseline justify-between gap-2">
            <span class="font-medium break-words">{{ h.heading ? `${h.title} · ${h.heading}` : h.title }}</span>
            <span class="text-xs text-base-content/70 tabular-nums">{{ t('knowledge.score', { score: h.score.toFixed(2) }) }}</span>
          </div>
          <span v-if="h.page" class="text-xs text-base-content/70">{{ t('knowledge.page', { n: h.page }) }}</span>
          <a v-if="h.url" :href="h.url" target="_blank" rel="noopener noreferrer" class="link font-mono text-xs break-all">{{ h.url }}</a>
          <p class="whitespace-pre-line break-words text-base-content/70">
            {{ h.text }}
          </p>
        </li>
      </ol>
      <p v-else-if="searched" class="mt-3 text-ui text-base-content/70">
        {{ t('knowledge.no_hits') }}
      </p>
    </Section>

    <ConfirmModal
      :handle="deleting"
      :title="t('knowledge.delete_document_title')"
      :body="t('knowledge.delete_document_body', { name: deleting.target.value?.title ?? '' })"
      :confirm-label="t('common.delete')"
      :cancel-label="t('common.cancel')"
    />
  </div>
</template>
