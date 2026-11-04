<script setup lang="ts">
import type { Column } from '#base/components/ui/types';
import DataTable from '#base/components/ui/DataTable.vue';
import PageHeader from '#base/components/ui/PageHeader.vue';
import StatusLabel from '#base/components/ui/StatusLabel.vue';
import { useDates } from '#base/composables/useDates';
import { totals } from '~/components/run/runView';
import { useUnits } from '~/composables/useUnits';
import { sampleRun } from '~/preview/samples';

const { t } = useI18n();
const route = useRoute();
const { when } = useDates();
const { money, seconds } = useUnits();

const run = sampleRun();
const sum = totals(run.events);
const rows = [{ id: run.id, startedAt: run.startedAt, seconds: sum.seconds, costCents: sum.costCents }];
const base = computed(() => `/o/${route.params.org}/projects/${route.params.project}/agents/${route.params.agent}/runs`);

const columns = computed<Column[]>(() => [
  { key: 'id', label: t('run.col_run'), mono: true },
  { key: 'status', label: t('run.status') },
  { key: 'startedAt', label: t('run.col_started'), muted: true },
  { key: 'seconds', label: t('run.duration'), align: 'end' },
  { key: 'costCents', label: t('run.cost'), align: 'end' },
]);

useHead({ title: () => `${t('agent.runs')} · OAgent` });
</script>

<template>
  <div class="flex flex-col gap-4">
    <PageHeader :title="t('agent.runs')" :description="t('run.list_hint')" :preview="t('preview.badge')">
      <template #status>
        <p role="note" class="flex items-center gap-1.5 text-xs text-base-content/70">
          <Icon name="lucide:flask-conical" size="14" class="shrink-0" aria-hidden="true" />
          {{ t('run.preview_note') }}
        </p>
      </template>
    </PageHeader>
    <DataTable :columns="columns" :rows="rows" row-key="id" :caption="t('run.samples_caption')">
      <template #cell-id="{ row }">
        <NuxtLink :to="`${base}/${row.id}`" class="link link-hover">
          {{ row.id }}
        </NuxtLink>
      </template>
      <template #cell-status>
        <StatusLabel tone="warning" :label="t('run.status_waiting')" />
      </template>
      <template #cell-startedAt="{ row }">
        <time :datetime="row.startedAt.toISOString()" class="tabular-nums">{{ when(row.startedAt) }}</time>
      </template>
      <template #cell-seconds="{ row }">
        {{ seconds(row.seconds) }}
      </template>
      <template #cell-costCents="{ row }">
        {{ money(row.costCents) }}
      </template>
    </DataTable>
  </div>
</template>
