<script setup lang="ts">
const props = defineProps<{ data: Record<string, unknown> }>();
const app = inject<any>('mcpApp');
const input = inject<Ref<Record<string, unknown>>>('mcpInput');

function rowsFrom(value: unknown): Record<string, unknown>[] {
  return Array.isArray(value) ? (value as Record<string, unknown>[]) : [];
}

const rows = ref<Record<string, unknown>[]>(rowsFrom(props.data?.webhooks));
const error = ref('');
const busy = ref(false);

async function refresh() {
  const orgID = (input?.value?.org_id as string) ?? (rows.value[0]?.orgId as string) ?? '';
  const projectID = (input?.value?.project_id as string) ?? (rows.value[0]?.projectId as string) ?? '';
  if (!orgID || !projectID) {
    error.value = 'missing org_id/project_id';
    return;
  }
  error.value = '';
  busy.value = true;
  try {
    const res = await app.callServerTool({ name: 'webhook_list', arguments: { org_id: orgID, project_id: projectID } });
    if (res.isError) {
      error.value = JSON.stringify(res.content ?? res);
      return;
    }
    const sc = res.structuredContent as any;
    rows.value = rowsFrom(sc?.webhooks ?? sc);
  } catch (err) {
    error.value = String(err);
  } finally {
    busy.value = false;
  }
}

const count = computed(() => rows.value.length);

function events(webhook: Record<string, unknown>): string {
  const types = webhook.eventTypes ?? webhook.event_types;
  return Array.isArray(types) ? types.join(', ') : String(types ?? '');
}
</script>

<template>
  <main class="card">
    <header><span class="dot" /><strong>Webhooks</strong><span class="muted"> · {{ count }}</span></header>

    <p v-if="count === 0" class="muted">
      No webhooks.
    </p>
    <table v-else class="list">
      <thead>
        <tr><th>url</th><th>events</th><th>active</th><th>id</th></tr>
      </thead>
      <tbody>
        <tr v-for="webhook in rows" :key="String(webhook.id)">
          <td class="mono">
            {{ webhook.url }}
          </td>
          <td>{{ events(webhook) }}</td>
          <td>{{ webhook.active }}</td>
          <td class="mono muted">
            {{ webhook.id }}
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
