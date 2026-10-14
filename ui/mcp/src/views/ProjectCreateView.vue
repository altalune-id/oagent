<script setup lang="ts">
const props = defineProps<{ data: Record<string, unknown> }>();
const app = inject<any>('mcpApp');
const input = inject<Ref<Record<string, unknown>>>('mcpInput');

const created = ref<Record<string, unknown> | null>((props.data?.project as Record<string, unknown>) ?? null);
const showForm = ref(!created.value && !input?.value?.name);
const orgID = ref((input?.value?.org_id as string) ?? '');
const slug = ref('');
const name = ref('');
const error = ref('');
const busy = ref(false);

async function submit() {
  error.value = '';
  busy.value = true;
  try {
    const res = await app.callServerTool({ name: 'project_create', arguments: { org_id: orgID.value, slug: slug.value, name: name.value } });
    if (res.isError) {
      error.value = JSON.stringify(res.content ?? res);
      return;
    }
    created.value = (res.structuredContent as any)?.project ?? (res.structuredContent as any) ?? null;
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
  error.value = '';
  showForm.value = true;
}
</script>

<template>
  <main class="card">
    <header><span class="dot" /><strong>Create project</strong></header>

    <form v-if="showForm" class="stack" @submit.prevent="submit">
      <label>Organization ID<input v-model="orgID" required placeholder="public id"></label>
      <label>Name<input v-model="name" required placeholder="Support Bot"></label>
      <label>Slug<input v-model="slug" placeholder="support-bot (generated when empty)"></label>
      <button type="submit" :disabled="busy">
        {{ busy ? "Creating…" : "Create" }}
      </button>
    </form>

    <template v-else>
      <p v-if="!created" class="muted">
        No project created.
      </p>
      <dl v-else>
        <dt>id</dt><dd>{{ created.id }}</dd>
        <dt>orgId</dt><dd>{{ created.orgId }}</dd>
        <dt>slug</dt><dd>{{ created.slug }}</dd>
        <dt>name</dt><dd>{{ created.name }}</dd>
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
