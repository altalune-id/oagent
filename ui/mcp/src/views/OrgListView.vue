<script setup lang="ts">
const props = defineProps<{ data: Record<string, unknown> }>();
const app = inject<any>('mcpApp');

function rowsFrom(value: unknown): Record<string, unknown>[] {
  return Array.isArray(value) ? (value as Record<string, unknown>[]) : [];
}

const rows = ref<Record<string, unknown>[]>(rowsFrom(props.data?.orgs));
const error = ref('');
const busy = ref(false);

async function refresh() {
  error.value = '';
  busy.value = true;
  try {
    const res = await app.callServerTool({ name: 'org_list', arguments: {} });
    if (res.isError) {
      error.value = JSON.stringify(res.content ?? res);
      return;
    }
    const sc = res.structuredContent as any;
    rows.value = rowsFrom(sc?.orgs ?? sc);
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
    <header><span class="dot" /><strong>Organizations</strong><span class="muted"> · {{ count }}</span></header>

    <p v-if="count === 0" class="muted">
      No organizations.
    </p>
    <table v-else class="list">
      <thead>
        <tr><th>slug</th><th>name</th><th>id</th></tr>
      </thead>
      <tbody>
        <tr v-for="org in rows" :key="String(org.id)">
          <td class="mono">
            {{ org.slug }}
          </td>
          <td>{{ org.name }}</td>
          <td class="mono muted">
            {{ org.id }}
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
