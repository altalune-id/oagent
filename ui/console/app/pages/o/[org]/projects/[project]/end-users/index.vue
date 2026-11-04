<script setup lang="ts">
import type { Column } from '#base/components/ui/types';
import type { EndUser } from '~/stores/endUsers';
import DataTable from '#base/components/ui/DataTable.vue';
import EmptyState from '#base/components/ui/EmptyState.vue';
import LoadError from '#base/components/ui/LoadError.vue';
import PageHeader from '#base/components/ui/PageHeader.vue';
import { useDates } from '#base/composables/useDates';
import { filterEndUsers, identityLabel } from '~/components/enduser/endUser';
import PreviewNote from '~/components/layout/PreviewNote.vue';
import { useProject } from '~/components/project/useProject';
import { useEndUsersStore } from '~/stores/endUsers';

const { t } = useI18n();
const route = useRoute();
const { when } = useDates();
const { project } = useProject();
const store = useEndUsersStore();
void store.ensureLoaded(project.value!.id);

const query = ref('');
const rows = computed(() => filterEndUsers(store.items, query.value));
const base = computed(() => `/o/${route.params.org}/projects/${route.params.project}/end-users`);

const columns = computed<Column[]>(() => [
  { key: 'name', label: t('endusers.col_name') },
  { key: 'identities', label: t('endusers.col_identities') },
  { key: 'threads', label: t('endusers.col_threads'), align: 'end' },
  { key: 'memories', label: t('endusers.col_memories'), align: 'end' },
  { key: 'lastSeen', label: t('endusers.col_last_seen'), muted: true },
]);

useHead({ title: () => `${t('endusers.title')} · ${project.value?.name ?? ''} · OAgent` });
</script>

<template>
  <div class="flex flex-col gap-4">
    <PageHeader :title="t('endusers.title')" :description="t('endusers.subtitle')" :preview="t('preview.badge')">
      <template #status>
        <PreviewNote :text="t('preview.local_note')" />
      </template>
    </PageHeader>

    <LoadError v-if="store.error" :error="store.error" :busy="store.loading" :retry-label="t('common.retry')" @retry="store.load(project!.id)" />
    <template v-else>
      <label v-if="store.items.length" class="input input-sm w-full text-ui sm:w-64">
        <Icon name="lucide:search" size="14" class="shrink-0" aria-hidden="true" />
        <span class="sr-only">{{ t('endusers.search') }}</span>
        <input v-model="query" type="search" :placeholder="t('endusers.search')">
      </label>

      <DataTable stacked :caption="t('endusers.title')" row-key="id" :columns="columns" :rows="rows" :loading="store.loading">
        <template #empty>
          <EmptyState v-if="query.trim()" icon="lucide:search-x" :title="t('endusers.empty_search', { query: query.trim() })" />
          <EmptyState v-else icon="lucide:users-round" :title="t('endusers.empty')" :body="t('endusers.empty_body')" />
        </template>
        <template #cell-name="{ row }">
          <NuxtLink :to="`${base}/${row.id}`" class="link link-hover font-medium">
            {{ row.name }}
          </NuxtLink>
        </template>
        <template #cell-identities="{ row }">
          <span class="flex flex-wrap gap-1">
            <span v-for="i in (row as EndUser).identities" :key="`${i.kind}:${i.value}`" class="badge badge-sm badge-ghost whitespace-nowrap">{{ identityLabel(i, t) }}</span>
          </span>
        </template>
        <template #cell-threads="{ row }">
          <span class="tabular-nums">{{ row.threads.length }}</span>
        </template>
        <template #cell-memories="{ row }">
          <span v-if="store.memoryOn" class="tabular-nums">{{ row.memories.length }}</span>
          <span v-else class="text-base-content/70">{{ t('endusers.memories_off') }}</span>
        </template>
        <template #cell-lastSeen="{ row }">
          {{ when(row.lastSeenAt) }}
        </template>
      </DataTable>
    </template>
  </div>
</template>
