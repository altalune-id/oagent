<script setup lang="ts">
const props = defineProps<{ data: Record<string, unknown> }>();
const app = inject<any>('mcpApp');
const input = inject<Ref<Record<string, unknown>>>('mcpInput');

function rowsFrom(value: unknown): Record<string, unknown>[] {
  return Array.isArray(value) ? (value as Record<string, unknown>[]) : [];
}

const rows = ref<Record<string, unknown>[]>(rowsFrom(props.data?.projects));
const error = ref('');
const busy = ref(false);

async function refresh() {
  const orgID = (input?.value?.org_id as string) ?? (rows.value[0]?.orgId as string) ?? '';
  if (!orgID) {
    error.value = 'missing org_id';
    return;
  }
  error.value = '';
  busy.value = true;
  try {
    const res = await app.callServerTool({ name: 'project_list', arguments: { org_id: orgID } });
    if (res.isError) {
      error.value = JSON.stringify(res.content ?? res);
      return;
    }
    const sc = res.structuredContent as any;
    rows.value = rowsFrom(sc?.projects ?? sc);
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
    <header><span class="dot" /><strong>Projects</strong><span class="muted"> · {{ count }}</span></header>

    <p v-if="count === 0" class="muted">
      No projects.
    </p>
    <table v-else class="list">
      <thead>
        <tr><th>slug</th><th>name</th><th>id</th></tr>
      </thead>
      <tbody>
        <tr v-for="project in rows" :key="String(project.id)">
          <td class="mono">
            {{ project.slug }}
          </td>
          <td>{{ project.name }}</td>
          <td class="mono muted">
            {{ project.id }}
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
