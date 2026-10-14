<script setup lang="ts">
import type { Ref } from 'vue';

const props = defineProps<{ data: Record<string, unknown> }>();
const app = inject<any>('mcpApp');
const input = inject<Ref<Record<string, unknown>>>('mcpInput');

const created = ref<Record<string, unknown> | null>((props.data?.org as Record<string, unknown>) ?? null);
const showForm = ref(!created.value && !input?.value?.name);
const slug = ref('');
const name = ref('');
const error = ref('');
const busy = ref(false);

async function submit() {
  error.value = '';
  busy.value = true;
  try {
    const res = await app.callServerTool({ name: 'org_create', arguments: { slug: slug.value, name: name.value } });
    if (res.isError) {
      error.value = JSON.stringify(res.content ?? res);
      return;
    }
    created.value = (res.structuredContent as any)?.org ?? (res.structuredContent as any) ?? null;
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
    <header><span class="dot" /><strong>Create organization</strong></header>

    <form v-if="showForm" class="stack" @submit.prevent="submit">
      <label>Name<input v-model="name" required placeholder="Acme"></label>
      <label>Slug<input v-model="slug" placeholder="acme (generated when empty)"></label>
      <button type="submit" :disabled="busy">
        {{ busy ? "Creating…" : "Create" }}
      </button>
    </form>

    <template v-else>
      <p v-if="!created" class="muted">
        No organization created.
      </p>
      <dl v-else>
        <dt>id</dt><dd>{{ created.id }}</dd>
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
