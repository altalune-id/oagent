<script setup lang="ts">
import { memberRows } from './members';

const props = defineProps<{ data: Record<string, unknown> }>();
const app = inject<any>('mcpApp');
const input = inject<Ref<Record<string, unknown>>>('mcpInput');

const rows = ref(memberRows(props.data));
const error = ref('');
const busy = ref(false);

async function refresh() {
  const orgID = (input?.value?.org_id as string) ?? '';
  if (!orgID) {
    error.value = 'missing org_id';
    return;
  }
  error.value = '';
  busy.value = true;
  try {
    const res = await app.callServerTool({ name: 'member_list', arguments: { org_id: orgID } });
    if (res.isError) {
      error.value = JSON.stringify(res.content ?? res);
      return;
    }
    rows.value = memberRows(res.structuredContent as Record<string, unknown>);
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
    <header><span class="dot" /><strong>Members</strong><span class="muted"> · {{ count }}</span></header>

    <p v-if="count === 0" class="muted">
      No members.
    </p>
    <table v-else class="list">
      <thead>
        <tr><th>name</th><th>email</th><th>role</th></tr>
      </thead>
      <tbody>
        <tr v-for="m in rows" :key="m.id">
          <td>{{ m.name }}</td>
          <td class="mono">
            {{ m.email }}
          </td>
          <td>{{ m.role }}</td>
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
