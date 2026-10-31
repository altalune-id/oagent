<script setup lang="ts">
import type { Ref } from 'vue';
import type { ShownError } from '#base/lib/apperror/message';
import type { ToolCaller } from './bridge';
import type { ListDescriptor } from './descriptor';
import ErrorText from '#base/components/ui/ErrorText.vue';
import ViewShell from '../components/ViewShell.vue';
import { t } from '../copy';
import { callTool } from './bridge';
import { pick, rowsAt } from './descriptor';
import RowTable from './RowTable.vue';

const { data, descriptor: d } = defineProps<{ data: Record<string, unknown>, descriptor: ListDescriptor }>();
const app = inject<ToolCaller>('mcpApp');
const input = inject<Ref<Record<string, unknown>>>('mcpInput', ref({}));

const rows = ref(rowsAt(data, d.rows));
const busy = ref(false);
const error = ref<ShownError | null>(null);

watch(() => data, (fresh) => {
  rows.value = rowsAt(fresh, d.rows);
});

async function load() {
  if (!app || !d.refresh || busy.value) {
    return;
  }
  busy.value = true;
  error.value = null;
  try {
    const out = await callTool(app, d.refresh.tool, pick(input.value, d.refresh.args));
    rows.value = rowsAt(out, d.rows);
  } catch (err) {
    error.value = { message: (err as Error).message };
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <ViewShell :title="d.title" :count="rows.length">
    <RowTable :rows="rows" :columns="d.columns" :row-id="d.key" :empty="d.empty" />
    <div v-if="d.refresh" class="flex gap-2">
      <button class="btn btn-ghost btn-sm" type="button" :disabled="busy" @click="load()">
        <span v-if="busy" class="loading loading-spinner loading-xs" aria-hidden="true" />
        {{ busy ? t('mcp.refreshing') : t('common.refresh') }}
      </button>
    </div>
    <ErrorText :error="error" />
  </ViewShell>
</template>
