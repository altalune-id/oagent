<script setup lang="ts">
import type { Column } from '#base/components/ui/types';
import type { ThreadFilter } from '~/components/thread/conversation';
import type { ThreadDetail, ThreadSummary } from '~/stores/threads';
import AppButton from '#base/components/ui/AppButton.vue';
import DataTable from '#base/components/ui/DataTable.vue';
import EmptyState from '#base/components/ui/EmptyState.vue';
import LoadError from '#base/components/ui/LoadError.vue';
import PageHeader from '#base/components/ui/PageHeader.vue';
import StatusLabel from '#base/components/ui/StatusLabel.vue';
import { useDates } from '#base/composables/useDates';
import { useAgentPage } from '~/components/agent/useAgentPage';
import { shownName } from '~/components/enduser/forget';
import PreviewNote from '~/components/layout/PreviewNote.vue';
import { channelFilters, filterFromQuery, filterQuery, filterThreads, runStatuses, runStatusTone } from '~/components/thread/conversation';
import { useThreadsStore } from '~/stores/threads';

const { t } = useI18n();
const lastLine = (th: ThreadSummary) => `${shownName(th.lastSpeaker, th.lastDeleted, t)}: ${th.lastDeleted ? t('chat.message_deleted') : th.lastText}`;
const route = useRoute();
const router = useRouter();
const { when } = useDates();
const { agent, base, usesSample } = await useAgentPage();
const store = useThreadsStore();
const uid = useId();
void store.ensureLoaded(agent.value.id, usesSample.value);

const filter = ref<ThreadFilter>(filterFromQuery(route.query));
watch(() => route.query, (query) => {
  const next = filterFromQuery(query);
  if (JSON.stringify(filterQuery(next)) !== JSON.stringify(filterQuery(filter.value))) {
    filter.value = next;
  }
});
watch(filter, (next) => {
  void router.replace({ query: filterQuery(next) });
}, { deep: true });
const rows = computed(() => filterThreads(store.threads, filter.value));
const filtered = computed(() => !!(filter.value.channel || filter.value.status || filter.value.query.trim()));
const kindIcon = { whatsapp_dm: 'lucide:message-circle', whatsapp_group: 'lucide:users-round', api: 'lucide:code', try: 'lucide:message-square-text' } as const;

const columns = computed<Column[]>(() => [
  { key: 'title', label: t('threads.col_person') },
  { key: 'kind', label: t('threads.col_channel') },
  { key: 'lastText', label: t('threads.col_last') },
  { key: 'status', label: t('run.status') },
  { key: 'updatedAt', label: t('threads.col_updated'), muted: true },
]);

function clear() {
  filter.value = { channel: '', status: '', query: '' };
}

useHead({ title: () => `${agent.value?.name ?? ''} · ${t('threads.title')} · OAgent` });
</script>

<template>
  <div class="flex flex-col gap-4">
    <PageHeader :title="t('threads.title')" :description="t('threads.subtitle')" :preview="t('preview.badge')">
      <template #status>
        <PreviewNote :text="t('threads.preview_note')" />
      </template>
    </PageHeader>

    <LoadError v-if="store.error" :error="store.error" :busy="store.loading" :retry-label="t('common.retry')" @retry="store.load(agent.id, usesSample)" />
    <template v-else>
      <div class="flex flex-wrap items-end gap-3">
        <div class="flex flex-col gap-1">
          <span :id="`${uid}-channel`" class="label text-xs text-base-content/70">{{ t('threads.col_channel') }}</span>
          <div class="flex flex-wrap items-center gap-1" role="group" :aria-labelledby="`${uid}-channel`">
            <AppButton
              v-for="c in channelFilters" :key="c" size="sm" :variant="filter.channel === c ? 'neutral' : 'outline'"
              :aria-pressed="filter.channel === c" @click="filter.channel = filter.channel === c ? '' : c"
            >
              <Icon v-if="filter.channel === c" name="lucide:check" size="14" aria-hidden="true" />
              {{ t(`threads.filter_${c}`) }}
            </AppButton>
          </div>
        </div>
        <label class="flex flex-col gap-1">
          <span class="label text-xs text-base-content/70">{{ t('run.status') }}</span>
          <select v-model="filter.status" class="select select-sm w-44 text-ui">
            <option value="">
              {{ t('threads.any_status') }}
            </option>
            <option v-for="s in runStatuses" :key="s" :value="s">
              {{ t(`runstatus.${s}`) }}
            </option>
          </select>
        </label>
        <label class="input input-sm w-full text-ui sm:w-64">
          <Icon name="lucide:search" size="14" class="shrink-0" aria-hidden="true" />
          <span class="sr-only">{{ t('threads.search') }}</span>
          <input v-model="filter.query" type="search" :placeholder="t('threads.search')">
        </label>
        <AppButton v-if="filtered" variant="ghost" size="sm" @click="clear()">
          <Icon name="lucide:x" size="14" aria-hidden="true" />
          {{ t('threads.clear') }}
        </AppButton>
      </div>

      <DataTable stacked :caption="t('threads.title')" row-key="id" :columns="columns" :rows="rows" :loading="store.loading">
        <template #empty>
          <EmptyState v-if="filtered" icon="lucide:filter-x" :title="t('threads.no_match')" :body="t('threads.no_match_body')">
            <template #actions>
              <AppButton variant="outline" size="sm" @click="clear()">
                {{ t('threads.clear') }}
              </AppButton>
            </template>
          </EmptyState>
          <EmptyState v-else icon="lucide:messages-square" :title="t('threads.empty')" :body="t('threads.empty_body')">
            <template #actions>
              <AppButton size="sm" :to="`${base}/try`">
                {{ t('try.title') }}
              </AppButton>
              <AppButton variant="outline" size="sm" :to="`${base}/channels`">
                {{ t('openwa.connect') }}
              </AppButton>
            </template>
          </EmptyState>
        </template>
        <template #cell-title="{ row }">
          <NuxtLink :to="`${base}/threads/${row.id}`" class="link link-hover inline-flex items-center gap-1.5 font-medium">
            <Icon :name="kindIcon[(row as ThreadDetail).kind]" size="14" class="shrink-0" aria-hidden="true" />
            {{ row.kind === 'api' ? t('threads.api_user', { id: row.title }) : row.kind === 'try' ? t('try.title') : row.title }}
          </NuxtLink>
        </template>
        <template #cell-kind="{ row }">
          <span class="badge badge-sm badge-ghost whitespace-nowrap">{{ t(`threads.kind_${row.kind}`) }}</span>
        </template>
        <template #cell-lastText="{ row }">
          <span class="block max-w-72 truncate text-base-content/70" :title="lastLine(row as ThreadSummary)">{{ lastLine(row as ThreadSummary) }}</span>
        </template>
        <template #cell-status="{ row }">
          <StatusLabel :tone="runStatusTone[(row as ThreadDetail).status]" :label="t(`runstatus.${row.status}`)" />
        </template>
        <template #cell-updatedAt="{ row }">
          <time :datetime="row.updatedAt.toISOString()" class="tabular-nums">{{ when(row.updatedAt) }}</time>
        </template>
      </DataTable>
    </template>
  </div>
</template>
