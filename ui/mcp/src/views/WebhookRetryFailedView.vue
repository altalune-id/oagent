<script setup lang="ts">
const props = defineProps<{ data: Record<string, unknown> }>();
const app = inject<any>('mcpApp');
const input = inject<Ref<Record<string, unknown>>>('mcpInput');

const orgID = ref((input?.value?.org_id as string) ?? '');
const projectID = ref((input?.value?.project_id as string) ?? '');
const id = ref((input?.value?.id as string) ?? '');
const requeued = ref<number | null>((props.data?.requeued as number) ?? null);
const error = ref('');
const busy = ref(false);

async function submit() {
  if (!orgID.value || !projectID.value || !id.value) {
    error.value = 'missing org_id/project_id/id';
    return;
  }
  error.value = '';
  busy.value = true;
  try {
    const res = await app.callServerTool({
      name: 'webhook_retry_failed',
      arguments: { org_id: orgID.value, project_id: projectID.value, id: id.value },
    });
    if (res.isError) {
      error.value = JSON.stringify(res.content ?? res);
      return;
    }
    const sc = res.structuredContent as any;
    requeued.value = Number(sc?.requeued ?? 0);
  } catch (err) {
    error.value = String(err);
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <main class="card">
    <header><span class="dot" /><strong>Retry failed deliveries</strong></header>

    <form class="stack" @submit.prevent="submit">
      <label>Organization ID<input v-model="orgID" required placeholder="public id"></label>
      <label>Project ID<input v-model="projectID" required placeholder="public id"></label>
      <label>Webhook ID<input v-model="id" required placeholder="public id"></label>
      <button type="submit" :disabled="busy">
        {{ busy ? "Retrying…" : "Retry failed" }}
      </button>
    </form>

    <p v-if="requeued !== null" class="muted">
      Requeued <strong>{{ requeued }}</strong> deliveries.
    </p>
    <p v-if="error" class="error">
      {{ error }}
    </p>
  </main>
</template>
