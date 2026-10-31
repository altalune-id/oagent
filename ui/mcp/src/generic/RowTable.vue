<script setup lang="ts">
import type { Field } from './descriptor';
import { rowKey, valueAt } from './descriptor';
import FieldValue from './FieldValue.vue';

const { rows, columns, rowId, empty } = defineProps<{
  rows: Record<string, unknown>[],
  columns: Field[],
  rowId: string | string[],
  empty: string,
}>();
</script>

<template>
  <p v-if="rows.length === 0" class="text-sm text-base-content/60">
    {{ empty }}
  </p>
  <div v-else class="overflow-x-auto">
    <table class="table table-sm">
      <thead>
        <tr>
          <th v-for="c in columns" :key="c.key">
            {{ c.label }}
          </th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="row in rows" :key="rowKey(row, rowId)">
          <td v-for="c in columns" :key="c.key">
            <FieldValue :field="c" :value="valueAt(row, c.key)" />
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
