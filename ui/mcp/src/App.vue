<script setup lang="ts">
import { App } from '@modelcontextprotocol/ext-apps';
import { resolveView } from './views';

const app = new App({ name: 'oagent', version: '0.1.0' });
provide('mcpApp', app);

const input = ref<Record<string, unknown>>({});
provide('mcpInput', input);

const state = ref<'connecting' | 'ready' | 'empty' | 'error'>('connecting');
const data = ref<Record<string, unknown>>({});
const view = shallowRef(resolveView(''));
const message = ref('');

onMounted(async () => {
  app.ontoolinput = (params: any) => {
    input.value = params?.arguments ?? {};
  };

  app.ontoolresult = (params: any) => {
    if (params?.isError) {
      state.value = 'error';
      message.value = JSON.stringify(params.content ?? params);
      return;
    }
    data.value = params?.structuredContent ?? {};
    state.value = Object.keys(data.value).length > 0 ? 'ready' : 'empty';
  };

  try {
    await app.connect();
    const tool = (app.getHostContext() as any)?.toolInfo?.tool?.name ?? '';
    view.value = resolveView(tool);
    state.value = 'empty';
  } catch (err) {
    state.value = 'error';
    message.value = String(err);
  }
});
</script>

<template>
  <component :is="view" v-if="state === 'ready'" :data="data" />
  <main v-else class="card">
    <header><span class="dot" /><strong>oagent</strong></header>
    <p v-if="state === 'connecting'" class="muted">
      Connecting…
    </p>
    <p v-else-if="state === 'error'" class="error">
      {{ message }}
    </p>
    <p v-else class="muted">
      Waiting for a tool result…
    </p>
  </main>
</template>

<style>
:root {
  color-scheme: light dark;
}
body {
  margin: 0;
  font:
    14px/1.5 system-ui,
    -apple-system,
    sans-serif;
  background: transparent;
}
.card {
  border: 1px solid rgba(128, 128, 128, 0.3);
  border-radius: 10px;
  padding: 16px;
  max-width: 560px;
}
header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
}
.dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #3b82f6;
}
.muted {
  opacity: 0.6;
}
.error {
  color: #dc2626;
}
dl {
  display: grid;
  grid-template-columns: auto 1fr;
  gap: 4px 16px;
  margin: 12px 0 0;
}
dt {
  opacity: 0.6;
}
dd {
  margin: 0;
  font-family: ui-monospace, SFMono-Regular, monospace;
}
pre {
  margin: 0;
  overflow: auto;
  font-family: ui-monospace, SFMono-Regular, monospace;
}
.stack {
  display: grid;
  gap: 10px;
}
.stack label {
  display: grid;
  gap: 4px;
  font-size: 12px;
  opacity: 0.8;
}
.stack input {
  padding: 6px 8px;
  border: 1px solid rgba(128, 128, 128, 0.35);
  border-radius: 6px;
  font: inherit;
}
.stack button {
  justify-self: start;
  padding: 6px 14px;
  border: 0;
  border-radius: 6px;
  background: #3b82f6;
  color: #fff;
  font: inherit;
  cursor: pointer;
}
.stack button:disabled {
  opacity: 0.6;
  cursor: default;
}
.ghost {
  margin-top: 12px;
  padding: 6px 12px;
  border: 1px solid rgba(128, 128, 128, 0.4);
  border-radius: 6px;
  background: transparent;
  color: inherit;
  font: inherit;
  cursor: pointer;
}
.mono {
  font-family: ui-monospace, SFMono-Regular, monospace;
}
.list {
  width: 100%;
  border-collapse: collapse;
  margin-bottom: 12px;
  font-size: 13px;
}
.list th {
  text-align: left;
  font-weight: 600;
  opacity: 0.6;
  padding: 4px 8px 4px 0;
}
.list td {
  padding: 4px 8px 4px 0;
  border-top: 1px solid rgba(128, 128, 128, 0.2);
}
</style>
