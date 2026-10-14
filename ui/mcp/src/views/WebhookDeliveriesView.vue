<script setup lang="ts">
const props = defineProps<{ data: Record<string, unknown> }>();
const app = inject<any>('mcpApp');
const input = inject<Ref<Record<string, unknown>>>('mcpInput');

function rowsFrom(value: unknown): Record<string, unknown>[] {
  return Array.isArray(value) ? (value as Record<string, unknown>[]) : [];
}

const rows = ref<Record<string, unknown>[]>(rowsFrom(props.data?.deliveries));
const error = ref('');
const busy = ref(false);

async function refresh() {
  const orgID = (input?.value?.org_id as string) ?? '';
  const projectID = (input?.value?.project_id as string) ?? '';
  const id = (input?.value?.id as string) ?? '';
  if (!orgID || !projectID || !id) {
    error.value = 'missing org_id/project_id/id';
    return;
  }
  error.value = '';
  busy.value = true;
  try {
    const res = await app.callServerTool({ name: 'webhook_deliveries', arguments: { org_id: orgID, project_id: projectID, id } });
    if (res.isError) {
      error.value = JSON.stringify(res.content ?? res);
      return;
    }
    const sc = res.structuredContent as any;
    rows.value = rowsFrom(sc?.deliveries ?? sc);
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
    <header><span class="dot" /><strong>Deliveries</strong><span class="muted"> · {{ count }}</span></header>

    <p v-if="count === 0" class="muted">
      No deliveries.
    </p>
    <table v-else class="list">
      <thead>
        <tr><th>event</th><th>status</th><th>attempt</th><th>created</th></tr>
      </thead>
      <tbody>
        <tr v-for="delivery in rows" :key="String(delivery.id)">
          <td class="mono">
            {{ delivery.eventType }}
          </td>
          <td>{{ delivery.status }}</td>
          <td>{{ delivery.attempt }}</td>
          <td class="muted">
            {{ delivery.createdAt }}
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
