<script setup lang="ts">
const props = defineProps<{ data: Record<string, unknown> }>();
const app = inject<any>('mcpApp');
const input = inject<Ref<Record<string, unknown>>>('mcpInput');

const created = ref<Record<string, unknown> | null>((props.data?.agent as Record<string, unknown>) ?? null);
const showForm = ref(!created.value && !input?.value?.name);
const orgID = ref((input?.value?.org_id as string) ?? '');
const projectID = ref((input?.value?.project_id as string) ?? '');
const slug = ref('');
const name = ref('');
const description = ref('');
const error = ref('');
const busy = ref(false);

async function submit() {
  error.value = '';
  busy.value = true;
  try {
    const res = await app.callServerTool({
      name: 'agent_create',
      arguments: { org_id: orgID.value, project_id: projectID.value, slug: slug.value, name: name.value, description: description.value },
    });
    if (res.isError) {
      error.value = JSON.stringify(res.content ?? res);
      return;
    }
    created.value = (res.structuredContent as any)?.agent ?? (res.structuredContent as any) ?? null;
    showForm.value = false;
  } catch (err) {
    error.value = String(err);
  } finally {
    busy.value = false;
  }
}

function again() {
  created.value = null;
  slug.value = '';
  name.value = '';
  description.value = '';
  error.value = '';
  showForm.value = true;
}
</script>

<template>
  <main class="card">
    <header><span class="dot" /><strong>Create agent</strong></header>

    <form v-if="showForm" class="stack" @submit.prevent="submit">
      <label>Organization ID<input v-model="orgID" required placeholder="public id"></label>
      <label>Project ID<input v-model="projectID" required placeholder="public id"></label>
      <label>Name<input v-model="name" required placeholder="Support Bot"></label>
      <label>Slug<input v-model="slug" placeholder="support-bot (generated when empty)"></label>
      <label>Description<textarea v-model="description" rows="2" placeholder="What this agent does" /></label>
      <button type="submit" :disabled="busy">
        {{ busy ? "Creating…" : "Create" }}
      </button>
    </form>

    <template v-else>
      <p v-if="!created" class="muted">
        No agent created.
      </p>
      <dl v-else>
        <dt>id</dt><dd>{{ created.id }}</dd>
        <dt>projectId</dt><dd>{{ created.projectId }}</dd>
        <dt>slug</dt><dd>{{ created.slug }}</dd>
        <dt>name</dt><dd>{{ created.name }}</dd>
        <dt>description</dt><dd>{{ created.description }}</dd>
      </dl>
      <button class="ghost" type="button" @click="again">
        Create another
      </button>
    </template>

    <p v-if="error" class="error">
      {{ error }}
    </p>
  </main>
</template>
