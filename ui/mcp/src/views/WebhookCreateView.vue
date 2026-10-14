<script setup lang="ts">
const props = defineProps<{ data: Record<string, unknown> }>();
const app = inject<any>('mcpApp');
const input = inject<Ref<Record<string, unknown>>>('mcpInput');

const created = ref<Record<string, unknown> | null>((props.data?.webhook as Record<string, unknown>) ?? null);
const secret = ref<string>((props.data?.secret as string) ?? '');
const showForm = ref(!created.value && !input?.value?.url);
const orgID = ref((input?.value?.org_id as string) ?? '');
const projectID = ref((input?.value?.project_id as string) ?? '');
const url = ref('');
const description = ref('');
const eventTypes = ref('agent.created, agent.updated');
const error = ref('');
const busy = ref(false);

async function submit() {
  error.value = '';
  busy.value = true;
  try {
    const types = eventTypes.value.split(',').map(part => part.trim()).filter(Boolean);
    const res = await app.callServerTool({
      name: 'webhook_create',
      arguments: { org_id: orgID.value, project_id: projectID.value, url: url.value, description: description.value, event_types: types },
    });
    if (res.isError) {
      error.value = JSON.stringify(res.content ?? res);
      return;
    }
    const sc = res.structuredContent as any;
    created.value = sc?.webhook ?? null;
    secret.value = sc?.secret ?? '';
    showForm.value = false;
  } catch (err) {
    error.value = String(err);
  } finally {
    busy.value = false;
  }
}

function again() {
  created.value = null;
  secret.value = '';
  url.value = '';
  description.value = '';
  error.value = '';
  showForm.value = true;
}
</script>

<template>
  <main class="card">
    <header><span class="dot" /><strong>Create webhook</strong></header>

    <form v-if="showForm" class="stack" @submit.prevent="submit">
      <label>Organization ID<input v-model="orgID" required placeholder="public id"></label>
      <label>Project ID<input v-model="projectID" required placeholder="public id"></label>
      <label>URL<input v-model="url" required placeholder="https://example.com/hook"></label>
      <label>Description<input v-model="description" placeholder="optional"></label>
      <label>Event types<input v-model="eventTypes" placeholder="agent.created, agent.updated"></label>
      <button type="submit" :disabled="busy">
        {{ busy ? "Creating…" : "Create" }}
      </button>
    </form>

    <template v-else>
      <p v-if="!created" class="muted">
        No webhook created.
      </p>
      <dl v-else>
        <dt>id</dt><dd>{{ created.id }}</dd>
        <dt>projectId</dt><dd>{{ created.projectId }}</dd>
        <dt>url</dt><dd>{{ created.url }}</dd>
      </dl>
      <p v-if="secret">
        <strong>Secret (shown once):</strong> <span class="mono">{{ secret }}</span>
      </p>
      <button class="ghost" type="button" @click="again">
        Create another
      </button>
    </template>

    <p v-if="error" class="error">
      {{ error }}
    </p>
  </main>
</template>
