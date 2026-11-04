<script setup lang="ts">
import type { Column } from '#base/components/ui/types';
import type { RunStatus, RunSummary } from '~/stores/threads';
import DataTable from '#base/components/ui/DataTable.vue';
import EmptyState from '#base/components/ui/EmptyState.vue';
import LoadError from '#base/components/ui/LoadError.vue';
import PageHeader from '#base/components/ui/PageHeader.vue';
import StatusLabel from '#base/components/ui/StatusLabel.vue';
import { useDates } from '#base/composables/useDates';
import { useAgentPage } from '~/components/agent/useAgentPage';
import { shownName } from '~/components/enduser/forget';
import PreviewNote from '~/components/layout/PreviewNote.vue';
import { runStatuses, runStatusTone } from '~/components/thread/conversation';
import { useUnits } from '~/composables/useUnits';
import { useThreadsStore } from '~/stores/threads';

const { t } = useI18n();
const runDetail = (r: RunSummary) => t(r.detail!.key, r.personDeleted ? { ...r.detail!.params, name: shownName('', true, t) } : r.detail!.params ?? {});
const { when } = useDates();
const { money, seconds, count } = useUnits();
const { agent, base, usesSample } = await useAgentPage();
const store = useThreadsStore();
void store.ensureLoaded(agent.value.id, usesSample.value);

const status = ref<RunStatus | ''>('');
const rows = computed(() => store.runs.filter(r => !status.value || r.status === status.value));

const columns = computed<Column[]>(() => [
  { key: 'id', label: t('run.col_run'), mono: true },
  { key: 'threadTitle', label: t('runs.col_thread') },
  { key: 'status', label: t('run.status') },
  { key: 'startedAt', label: t('run.col_started'), muted: true },
  { key: 'seconds', label: t('run.duration'), align: 'end' },
  { key: 'tokens', label: t('run.tokens'), align: 'end' },
  { key: 'costCents', label: t('run.cost'), align: 'end' },
]);

useHead({ title: () => `${agent.value?.name ?? ''} · ${t('agent.runs')} · OAgent` });
</script>

<template>
  <div class="flex flex-col gap-4">
    <PageHeader :title="t('agent.runs')" :description="t('run.list_hint')" :preview="t('preview.badge')">
      <template #status>
        <PreviewNote :text="t('run.preview_note')" />
      </template>
    </PageHeader>
    <LoadError v-if="store.error" :error="store.error" :busy="store.loading" :retry-label="t('common.retry')" @retry="store.load(agent.id, usesSample)" />
    <template v-else>
      <label class="flex flex-col gap-1 self-start">
        <span class="label text-xs text-base-content/70">{{ t('run.status') }}</span>
        <select v-model="status" class="select select-sm w-48 text-ui">
          <option value="">
            {{ t('threads.any_status') }}
          </option>
          <option v-for="s in runStatuses" :key="s" :value="s">
            {{ t(`runstatus.${s}`) }}
          </option>
        </select>
      </label>
      <DataTable stacked :columns="columns" :rows="rows" row-key="id" :caption="t('run.samples_caption')" :loading="store.loading">
        <template #empty>
          <EmptyState icon="lucide:list-tree" :title="status ? t('runs.no_match') : t('runs.empty')" />
        </template>
        <template #cell-id="{ row }">
          <NuxtLink :to="`${base}/runs/${row.id}`" class="link link-hover">
            {{ row.id }}
          </NuxtLink>
        </template>
        <template #cell-threadTitle="{ row }">
          <NuxtLink :to="`${base}/threads/${row.threadId}`" class="link link-hover">
            {{ row.threadTitle }}
          </NuxtLink>
        </template>
        <template #cell-status="{ row }">
          <StatusLabel :tone="runStatusTone[(row as RunSummary).status]" :label="t(`runstatus.${row.status}`)" :detail="row.detail ? runDetail(row as RunSummary) : ''" />
        </template>
        <template #cell-startedAt="{ row }">
          <time :datetime="row.startedAt.toISOString()" class="tabular-nums">{{ when(row.startedAt) }}</time>
        </template>
        <template #cell-seconds="{ row }">
          {{ seconds(row.seconds) }}
        </template>
        <template #cell-tokens="{ row }">
          {{ count(row.tokens) }}
        </template>
        <template #cell-costCents="{ row }">
          {{ money(row.costCents) }}
        </template>
      </DataTable>
    </template>
  </div>
</template>
