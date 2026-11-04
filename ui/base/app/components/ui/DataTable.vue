<script setup lang="ts" generic="T extends object">
import type { Column, TableSort } from './types';
import { useSlots } from 'vue';

const { columns, rows, rowKey, caption, loading = false, stacked = false } = defineProps<{
  columns: Column[],
  rows: T[],
  rowKey: keyof T & string,
  caption: string,
  loading?: boolean,
  stacked?: boolean,
}>();

const slots = useSlots();
const sort = defineModel<TableSort>('sort');

function ariaSort(key: string): TableSort['dir'] | 'none' {
  return sort.value?.key === key ? sort.value.dir : 'none';
}

function toggleSort(key: string) {
  sort.value = { key, dir: sort.value?.key === key && sort.value.dir === 'ascending' ? 'descending' : 'ascending' };
}

function cell(row: T, key: string): unknown {
  return (row as Record<string, unknown>)[key];
}
</script>

<template>
  <div>
    <div v-if="!loading && !rows.length && slots.empty">
      <slot name="empty" />
    </div>
    <template v-else>
      <ul v-if="stacked && !loading" class="list rounded-box border border-base-300 bg-base-100 text-ui sm:hidden" :aria-label="caption">
        <li v-for="row in rows" :key="String(cell(row, rowKey))" class="list-row block py-2">
          <dl class="flex flex-col gap-1">
            <div v-for="col in columns" :key="col.key" class="flex min-w-0 items-start gap-3" :class="{ 'justify-end': col.hideLabel }">
              <dt v-if="!col.hideLabel" class="w-28 shrink-0 text-xs text-base-content/70">
                {{ col.label }}
              </dt>
              <dd class="min-w-0 flex-1 break-words" :class="{ 'font-mono text-xs': col.mono, 'text-base-content/70': col.muted, 'flex-none': col.hideLabel }">
                <slot :name="`cell-${col.key}`" :row="row">
                  {{ cell(row, col.key) }}
                </slot>
              </dd>
            </div>
          </dl>
        </li>
      </ul>
      <div class="max-w-full overflow-x-auto rounded-box border border-base-300 bg-base-100" :class="{ 'max-sm:hidden': stacked && !loading }">
        <table class="table table-sm text-ui" :aria-busy="loading || undefined">
          <caption class="sr-only">
            {{ caption }}
          </caption>
          <thead class="text-base-content/70">
            <tr>
              <th
                v-for="col in columns" :key="col.key" scope="col" class="bg-base-200 text-xs font-medium"
                :class="{ 'text-end': col.align === 'end' }" :aria-sort="col.sortable ? ariaSort(col.key) : undefined"
              >
                <button v-if="col.sortable" type="button" class="inline-flex items-center gap-1 hover:underline" @click="toggleSort(col.key)">
                  {{ col.label }}<Icon name="lucide:arrow-down-up" size="12" aria-hidden="true" />
                </button>
                <span v-else :class="{ 'sr-only': col.hideLabel }">{{ col.label }}</span>
              </th>
            </tr>
          </thead>
          <tbody v-if="loading">
            <tr v-for="n in 3" :key="n">
              <td v-for="col in columns" :key="col.key">
                <div class="skeleton h-4 w-24" />
              </td>
            </tr>
          </tbody>
          <tbody v-else>
            <tr v-for="row in rows" :key="String(cell(row, rowKey))">
              <td v-for="col in columns" :key="col.key" :class="{ 'text-end tabular-nums': col.align === 'end', 'font-mono text-xs': col.mono, 'text-base-content/70': col.muted }">
                <slot :name="`cell-${col.key}`" :row="row">
                  {{ cell(row, col.key) }}
                </slot>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </template>
  </div>
</template>
