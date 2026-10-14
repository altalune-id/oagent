<script setup lang="ts">
const props = defineProps<{ data: Record<string, unknown> }>();
const app = inject<any>('mcpApp');
const input = inject<Ref<Record<string, unknown>>>('mcpInput');

function rowsFrom(value: unknown): Record<string, unknown>[] {
  return Array.isArray(value) ? (value as Record<string, unknown>[]) : [];
}

const rows = ref<Record<string, unknown>[]>(rowsFrom(props.data?.agents));
const error = ref('');
const busy = ref(false);

async function refresh() {
  const projectID = (input?.value?.project_id as string) ?? (rows.value[0]?.projectId as string) ?? '';
  if (!projectID) {
    error.value = 'missing project_id';
    return;
  }
  error.value = '';
  busy.value = true;
  try {
    const res = await app.callServerTool({ name: 'agent_list', arguments: { project_id: projectID } });
    if (res.isError) {
      error.value = JSON.stringify(res.content ?? res);
      return;
    }
    const sc = res.structuredContent as any;
    rows.value = rowsFrom(sc?.agents ?? sc);
  } catch (err) {
    error.value = String(err);
  } finally {
    busy.value = false;
  }
}

const count = computed(() => rows.value.length);
</script>

<template>
  <main class="card">
    <header><span class="dot" /><strong>Agents</strong><span class="muted"> · {{ count }}</span></header>

    <p v-if="count === 0" class="muted">
      No agents.
    </p>
    <table v-else class="list">
      <thead>
        <tr><th>slug</th><th>name</th><th>description</th></tr>
      </thead>
      <tbody>
        <tr v-for="ag in rows" :key="String(ag.id)">
          <td class="mono">
            {{ ag.slug }}
          </td>
          <td>{{ ag.name }}</td>
          <td class="muted">
            {{ ag.description }}
          </td>
        </tr>
      </tbody>
    </table>

    <button class="ghost" type="button" :disabled="busy" @click="refresh">
      {{ busy ? "Refreshing…" : "Refresh" }}
    </button>
    <p v-if="error" class="error">
      {{ error }}
    </p>
  </main>
</template>
