<script setup lang="ts">
import type { Ref } from 'vue';

const props = defineProps<{ data: Record<string, unknown> }>();
const app = inject<any>('mcpApp');
const input = inject<Ref<Record<string, unknown>>>('mcpInput');

const argID = ref((input?.value?.id as string) ?? '');
const org = ref<Record<string, unknown> | null>((props.data?.org as Record<string, unknown>) ?? null);
const showForm = ref(!org.value && !argID.value);
const id = ref('');
const error = ref('');

async function submit() {
  error.value = '';
  try {
    const res = await app.callServerTool({ name: 'org_get', arguments: { id: id.value } });
    if (res.isError) {
      error.value = JSON.stringify(res.content ?? res);
      return;
    }
    org.value = (res.structuredContent as any)?.org ?? (res.structuredContent as any) ?? null;
    argID.value = id.value;
    showForm.value = false;
  } catch (err) {
    error.value = String(err);
  }
}

function again() {
  org.value = null;
  argID.value = '';
  id.value = '';
  error.value = '';
  showForm.value = true;
}
</script>

<template>
  <main class="card">
    <header><span class="dot" /><strong>Organization</strong></header>

    <form v-if="showForm" class="stack" @submit.prevent="submit">
      <label>ID<input v-model="id" required placeholder="public id"></label>
      <button type="submit">
        Get
      </button>
    </form>

    <template v-else>
      <p v-if="!org" class="muted">
        No organization found for <span class="mono">{{ argID }}</span>.
      </p>
      <dl v-else>
        <dt>id</dt><dd>{{ org.id }}</dd>
        <dt>slug</dt><dd>{{ org.slug }}</dd>
        <dt>name</dt><dd>{{ org.name }}</dd>
      </dl>
      <button class="ghost" type="button" @click="again">
        Look up another
      </button>
    </template>

    <p v-if="error" class="error">
      {{ error }}
    </p>
  </main>
</template>
