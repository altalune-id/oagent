<script setup lang="ts">
import type { Column } from '#base/components/ui/types';
import type { EndUser } from '~/stores/endUsers';
import DataTable from '#base/components/ui/DataTable.vue';
import EmptyState from '#base/components/ui/EmptyState.vue';
import LoadError from '#base/components/ui/LoadError.vue';
import PageHeader from '#base/components/ui/PageHeader.vue';
import { useDates } from '#base/composables/useDates';
import { identityKindLabel, personLabel } from '~/components/enduser/label';
import { useProject } from '~/components/project/useProject';
import { useEndUsersStore } from '~/stores/endUsers';

const { t } = useI18n();
const route = useRoute();
const { when } = useDates();
const { project } = useProject();
const store = useEndUsersStore();
void store.ensureLoaded(project.value!.id);

const query = ref('');
const searched = ref('');
let timer: ReturnType<typeof setTimeout> | undefined;
watch(query, (q) => {
  clearTimeout(timer);
  timer = setTimeout(() => {
    searched.value = q.trim();
    void store.load(project.value!.id, searched.value);
  }, 300);
});
onBeforeUnmount(() => clearTimeout(timer));

const base = computed(() => `/o/${route.params.org}/projects/${route.params.project}/end-users`);
const label = (u: EndUser) => personLabel(u, t);

const columns = computed<Column[]>(() => [
  { key: 'name', label: t('endusers.col_name') },
  { key: 'identity', label: t('endusers.col_identity') },
  { key: 'threadCount', label: t('endusers.col_threads'), align: 'end' },
  { key: 'firstSeen', label: t('endusers.col_first_seen'), muted: true },
  { key: 'lastSeen', label: t('endusers.col_last_seen'), muted: true },
]);

useHead({ title: () => `${t('endusers.title')} · ${project.value?.name ?? ''} · OAgent` });
</script>

<template>
  <div class="flex flex-col gap-4">
    <PageHeader :title="t('endusers.title')" :description="t('endusers.subtitle')" />

    <label class="input input-sm w-full text-ui sm:w-64">
      <Icon name="lucide:search" size="14" class="shrink-0" aria-hidden="true" />
      <span class="sr-only">{{ t('endusers.search') }}</span>
      <input v-model="query" type="search" :placeholder="t('endusers.search')">
    </label>

    <LoadError v-if="store.error" :error="store.error" :busy="store.loading" :retry-label="t('common.retry')" @retry="store.load(project!.id, searched)" />
    <DataTable v-else stacked :caption="t('endusers.title')" row-key="id" :columns="columns" :rows="store.items" :loading="store.loading">
      <template #empty>
        <EmptyState v-if="searched" icon="lucide:search-x" :title="t('endusers.empty_search', { query: searched })" />
        <EmptyState v-else icon="lucide:users-round" :title="t('endusers.empty')" :body="t('endusers.empty_body')" />
      </template>
      <template #cell-name="{ row }">
        <NuxtLink :to="`${base}/${row.id}`" class="link link-hover font-medium">
          {{ label(row as EndUser) }}
        </NuxtLink>
      </template>
      <template #cell-identity="{ row }">
        <span class="badge badge-sm badge-ghost whitespace-nowrap">{{ identityKindLabel((row as EndUser).identityKind, t) }}<template v-if="row.identityHint"> …{{ row.identityHint }}</template></span>
      </template>
      <template #cell-threadCount="{ row }">
        <span class="tabular-nums">{{ row.threadCount }}</span>
      </template>
      <template #cell-firstSeen="{ row }">
        {{ row.firstSeenAt ? when(row.firstSeenAt) : '—' }}
      </template>
      <template #cell-lastSeen="{ row }">
        {{ row.lastSeenAt ? when(row.lastSeenAt) : '—' }}
      </template>
    </DataTable>
  </div>
</template>
