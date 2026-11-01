<script setup lang="ts" generic="T extends object">
import type { Column } from './types';
import { useSlots } from 'vue';

const { columns, rows, rowKey, caption, loading = false } = defineProps<{
  columns: Column[],
  rows: T[],
  rowKey: keyof T & string,
  caption: string,
  loading?: boolean,
}>();

const slots = useSlots();

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
      <div class="overflow-x-auto rounded-box border border-base-300 bg-base-100" :class="{ 'max-sm:hidden': slots['row-sm'] }">
        <table class="table table-sm" :aria-busy="loading || undefined">
          <caption class="sr-only">
            {{ caption }}
          </caption>
          <thead>
            <tr>
              <th v-for="col in columns" :key="col.key" scope="col" :class="{ 'text-right': col.align === 'end' }">
                {{ col.label }}
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
            <tr v-for="row in rows" :key="String(cell(row, rowKey))" class="row-hover">
              <td v-for="col in columns" :key="col.key" :class="{ 'text-right tabular-nums': col.align === 'end' }">
                <slot :name="`cell-${col.key}`" :row="row">
                  {{ cell(row, col.key) }}
                </slot>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <ul v-if="slots['row-sm']" class="list rounded-box border border-base-300 bg-base-100 sm:hidden" :aria-label="caption" :aria-busy="loading || undefined">
        <template v-if="loading">
          <li v-for="n in 3" :key="n" class="list-row">
            <div class="skeleton h-4 w-32" />
          </li>
        </template>
        <template v-else>
          <li v-for="row in rows" :key="String(cell(row, rowKey))" class="list-row">
            <slot name="row-sm" :row="row" />
          </li>
        </template>
      </ul>
    </template>
  </div>
</template>
