<script setup lang="ts">
import type { Column } from '#base/components/ui/types';
import type { ShownError } from '#base/lib/apperror/message';
import type { EndUserThread } from '~/stores/endUsers';
import type { Memory } from '~/stores/memories';
import AppButton from '#base/components/ui/AppButton.vue';
import { confirmHandle } from '#base/components/ui/confirmHandle';
import ConfirmModal from '#base/components/ui/ConfirmModal.vue';
import DataTable from '#base/components/ui/DataTable.vue';
import EmptyState from '#base/components/ui/EmptyState.vue';
import KeyValue from '#base/components/ui/KeyValue.vue';
import LoadError from '#base/components/ui/LoadError.vue';
import LoadingStatus from '#base/components/ui/LoadingStatus.vue';
import PageHeader from '#base/components/ui/PageHeader.vue';
import Section from '#base/components/ui/Section.vue';
import { useToast } from '#base/components/ui/toast';
import { useDates } from '#base/composables/useDates';
import { identityKindLabel, personLabel } from '~/components/enduser/label';
import { clearCopy, clip } from '~/components/enduser/memories';
import { useProject } from '~/components/project/useProject';
import { threadKind } from '~/components/thread/fromWire';
import { useCan } from '~/composables/useCan';
import { useAgentsStore } from '~/stores/agents';
import { useAuthStore } from '~/stores/auth';
import { useEndUsersStore } from '~/stores/endUsers';
import { useMemoriesStore } from '~/stores/memories';
import { useMemoryStoresStore } from '~/stores/memoryStores';

const { t } = useI18n();
const route = useRoute();
const { when, moment } = useDates();
const { project } = useProject();
const store = useEndUsersStore();
const agents = useAgentsStore();
void agents.ensureLoaded(project.value!.id);

const id = computed(() => route.params.endUser as string);
watch(id, next => void store.loadOne(next), { immediate: true });

const detail = computed(() => (store.detail?.user.id === id.value ? store.detail : null));
const name = computed(() => (detail.value ? personLabel(detail.value.user, t) : ''));
const base = computed(() => `/o/${route.params.org}/projects/${route.params.project}`);
const list = computed(() => `${base.value}/end-users`);
const facts = computed(() => {
  const u = detail.value?.user;
  if (!u) {
    return [];
  }
  return [
    { label: t('endusers.identity'), value: `${identityKindLabel(u.identityKind, t)}${u.identityHint ? ` …${u.identityHint}` : ''}` },
    { label: t('endusers.first_seen'), value: u.firstSeenAt ? moment(u.firstSeenAt) : '—' },
    { label: t('endusers.last_seen'), value: u.lastSeenAt ? moment(u.lastSeenAt) : '—' },
  ];
});
const agentOf = (th: EndUserThread) => agents.items.find(a => a.id === th.agentId);

const memories = useMemoriesStore();
const auth = useAuthStore();
const toast = useToast();
const canReadMemory = useCan('memory:read');
const canWriteMemory = useCan('memory:write');
void auth.load();
const memoryStores = useMemoryStoresStore();
watch([id, canReadMemory, () => auth.loaded], async ([next, can, ready]) => {
  if (ready && can) {
    await memoryStores.ensureLoaded(project.value!.id);
    const first = memoryStores.items[0]?.id;
    if (first) {
      void memories.load(first, next);
    }
  }
}, { immediate: true });
const storeFilter = computed({
  get: () => memories.storeId,
  set: (store: string) => void memories.load(store, id.value),
});
const memoryFilter = computed({
  get: () => memories.agentId,
  set: (agent: string) => void memories.load(memories.storeId, id.value, agent),
});
const agentById = (agentId: string) => agents.items.find(a => a.id === agentId);
const memoryColumns = computed<Column[]>(() => [
  { key: 'text', label: t('endusers.memory_col') },
  { key: 'agent', label: t('endusers.memory_agent_col') },
  { key: 'saved', label: t('endusers.memory_saved_col'), muted: true },
  { key: 'from', label: t('endusers.memory_from_col') },
  ...(canWriteMemory.value ? [{ key: 'menu', label: t('endusers.memory_actions'), hideLabel: true, align: 'end' as const }] : []),
]);
const clearText = computed(() => clearCopy(name.value, memories.total, memoryStores.items.find(s => s.id === memories.storeId)?.name ?? '', agentById(memories.agentId)?.name));
const deletingMemory = confirmHandle<Memory>(async (m) => {
  const done = await memories.remove(m.id);
  if (done === false) {
    throw memories.actionError;
  }
  toast.push(done === 'gone' ? 'info' : 'success', t(done === 'gone' ? 'endusers.memory_gone' : 'endusers.memory_deleted'));
}, e => e as ShownError);
const clearing = confirmHandle<true>(async () => {
  const n = await memories.clear();
  if (n === null) {
    throw memories.actionError;
  }
  toast.push('success', t('endusers.memories_cleared', { n }, n));
}, e => e as ShownError);

useHead({ title: () => `${name.value || t('endusers.title')} · ${t('endusers.title')} · OAgent` });
</script>

<template>
  <div v-if="store.detailError">
    <h1 class="sr-only">
      {{ t('endusers.title') }}
    </h1>
    <LoadError :error="store.detailError" :busy="store.detailLoading" :retry-label="t('common.retry')" @retry="store.loadOne(id)" />
  </div>
  <LoadingStatus v-else-if="store.detailLoading && !detail" :label="t('common.loading')" />
  <EmptyState v-else-if="!detail" icon="lucide:users-round" :title="t('error.end_user_not_found')" :heading-level="1">
    <template #actions>
      <AppButton variant="outline" size="sm" :to="list">
        {{ t('endusers.title') }}
      </AppButton>
    </template>
  </EmptyState>
  <div v-else class="flex flex-col gap-4">
    <PageHeader :title="name" />

    <Section :title="t('endusers.about')" :description="t('endusers.identities_desc')" flush>
      <KeyValue :items="facts" :copy-label="t('common.copy_label', { label: '{label}' })" />
    </Section>

    <Section v-if="canReadMemory" :title="t('endusers.memories')" :description="t('endusers.memories_desc', { n: memories.total }, memories.total)" flush>
      <template v-if="canWriteMemory && memories.total" #actions>
        <AppButton variant="outline" size="sm" @click="clearing.ask(true)">
          {{ t('endusers.memories_clear') }}
        </AppButton>
      </template>
      <p v-if="!memoryStores.loading && !memoryStores.items.length" class="px-4 py-3 text-ui text-base-content/70">
        {{ t('endusers.memories_no_stores') }}
        <NuxtLink :to="`${base}/memory-stores`" class="link">
          {{ t('memorystores.title') }}
        </NuxtLink>
      </p>
      <div v-else class="flex flex-col gap-3 p-4">
        <div class="flex flex-wrap gap-3">
          <label class="flex w-full max-w-xs flex-col gap-1">
            <span class="label text-ui text-base-content">{{ t('endusers.memories_store') }}</span>
            <select v-model="storeFilter" data-field="memory-store" class="select select-sm w-full text-ui">
              <option v-for="s in memoryStores.items" :key="s.id" :value="s.id">
                {{ s.name }}
              </option>
            </select>
          </label>
          <label class="flex w-full max-w-xs flex-col gap-1">
            <span class="label text-ui text-base-content">{{ t('endusers.memories_filter') }}</span>
            <select v-model="memoryFilter" data-field="memory-agent" class="select select-sm w-full text-ui">
              <option value="">
                {{ t('endusers.memories_all_agents') }}
              </option>
              <option v-for="a in agents.items" :key="a.id" :value="a.id">
                {{ a.name }}
              </option>
            </select>
          </label>
        </div>
        <LoadError v-if="memories.error" :error="memories.error" :busy="memories.loading" :retry-label="t('common.retry')" @retry="memories.load(memories.storeId, id, memories.agentId)" />
        <DataTable v-else stacked :loading="memories.loading && !memories.items.length" :caption="t('endusers.memories')" row-key="id" :columns="memoryColumns" :rows="memories.items">
          <template #empty>
            <EmptyState icon="lucide:brain" :heading-level="3" :title="t('endusers.memories_empty')" :body="t('endusers.memories_empty_body')" />
          </template>
          <template #cell-text="{ row }">
            <span class="break-words">{{ row.text }}</span>
          </template>
          <template #cell-agent="{ row }">
            {{ agentById(row.agentId)?.name ?? '' }}
          </template>
          <template #cell-saved="{ row }">
            <span :title="row.createdAt ? moment(row.createdAt) : undefined">{{ row.createdAt ? when(row.createdAt) : '—' }}</span>
          </template>
          <template #cell-from="{ row }">
            <NuxtLink v-if="row.sourceRunId && agentById(row.agentId)" :to="`${base}/agents/${agentById(row.agentId)!.slug}/runs/${row.sourceRunId}`" class="link inline-flex min-h-6 items-center">
              {{ t('endusers.view_run') }}
            </NuxtLink>
            <span v-else class="text-base-content/70">{{ t('endusers.run_gone') }}</span>
          </template>
          <template #cell-menu="{ row }">
            <AppButton variant="ghost" size="xs" square class="max-sm:size-11" :aria-label="t('endusers.memory_delete', { text: clip(row.text, 60) })" :title="t('endusers.memory_delete', { text: clip(row.text, 60) })" @click="deletingMemory.ask(row as Memory)">
              <Icon name="lucide:trash-2" size="14" class="text-error" aria-hidden="true" />
            </AppButton>
          </template>
        </DataTable>
        <AppButton v-if="memories.nextPageToken && !memories.error" variant="outline" size="sm" class="self-start" :busy="memories.loading" @click="memories.loadMore()">
          {{ t('approvals.load_more') }}
        </AppButton>
      </div>
    </Section>

    <Section :title="t('endusers.threads')" flush>
      <p v-if="!detail.threads.length" class="px-4 py-3 text-ui text-base-content/70">
        {{ t('endusers.threads_empty') }}
      </p>
      <ul v-else class="divide-y divide-base-300">
        <li v-for="th in detail.threads" :key="th.id" class="flex flex-wrap items-center gap-x-3 gap-y-1 px-4 py-2 text-ui">
          <NuxtLink v-if="agentOf(th)" :to="`${base}/agents/${agentOf(th)!.slug}/threads/${th.id}`" class="link link-hover font-mono text-xs">
            {{ th.id }}
          </NuxtLink>
          <span v-else class="font-mono text-xs">{{ th.id }}</span>
          <span class="badge badge-sm badge-ghost">{{ t(`threads.kind_${threadKind(th.kind)}`) }}</span>
          <span class="text-base-content/70">{{ agentOf(th)?.name ?? t('endusers.agent_gone') }}<template v-if="th.lastActivityAt"> · {{ when(th.lastActivityAt) }}</template></span>
        </li>
      </ul>
    </Section>

    <ConfirmModal
      v-if="canWriteMemory"
      :handle="deletingMemory"
      :title="t('endusers.memory_delete_title')"
      :body="deletingMemory.target.value ? clip(deletingMemory.target.value.text, 120) : ''"
      :confirm-label="t('common.delete')"
      :cancel-label="t('common.cancel')"
    />
    <ConfirmModal
      v-if="canWriteMemory"
      :handle="clearing"
      :title="t(clearText.title.key, clearText.title.params, clearText.title.count ?? 0)"
      :body="t(clearText.body.key, clearText.body.params)"
      :confirm-label="t('endusers.memories_clear')"
      :cancel-label="t('common.cancel')"
    />
  </div>
</template>
