<script setup lang="ts">
import type { Column } from '#base/components/ui/types';
import DataTable from '#base/components/ui/DataTable.vue';
import PageHeader from '#base/components/ui/PageHeader.vue';
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
  { key: 'id', label: t('run.col_run') },
  { key: 'status', label: t('run.status') },
  { key: 'startedAt', label: t('run.col_started') },
  { key: 'seconds', label: t('run.duration'), align: 'end' },
  { key: 'costCents', label: t('run.cost'), align: 'end' },
]);

useHead({ title: () => `${t('agent.runs')} · OAgent` });
</script>

<template>
  <div class="space-y-6">
    <PageHeader :title="t('agent.runs')" :description="t('run.list_hint')" :preview="t('preview.badge')" />
    <div role="note" class="alert alert-info alert-soft">
      <Icon name="lucide:flask-conical" size="18" />
      <span>{{ t('run.preview_note') }}</span>
    </div>
    <DataTable :columns="columns" :rows="rows" row-key="id" :caption="t('run.samples_caption')">
      <template #cell-id="{ row }">
        <NuxtLink :to="`${base}/${row.id}`" class="link link-hover font-mono text-xs">
          {{ row.id }}
        </NuxtLink>
      </template>
      <template #cell-status>
        <span class="badge badge-soft badge-warning badge-sm">{{ t('run.status_waiting') }}</span>
      </template>
      <template #cell-startedAt="{ row }">
        <time :datetime="row.startedAt.toISOString()">{{ when(row.startedAt) }}</time>
      </template>
      <template #cell-seconds="{ row }">
        {{ seconds(row.seconds) }}
      </template>
      <template #cell-costCents="{ row }">
        {{ money(row.costCents) }}
      </template>
      <template #row-sm="{ row }">
        <div class="list-col-grow min-w-0">
          <NuxtLink :to="`${base}/${row.id}`" class="link link-hover font-mono text-xs break-all">
            {{ row.id }}
          </NuxtLink>
          <p class="text-xs text-base-content/70">
            {{ t('run.status_waiting') }} · {{ seconds(row.seconds) }} · {{ money(row.costCents) }}
          </p>
        </div>
      </template>
    </DataTable>
  </div>
</template>
