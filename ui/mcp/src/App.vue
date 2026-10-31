<script setup lang="ts">
import { App } from '@modelcontextprotocol/ext-apps';
import ErrorText from '#base/components/ui/ErrorText.vue';
import ViewShell from './components/ViewShell.vue';
import { t } from './copy';
import { errorText } from './generic/bridge';
import { applyTheme } from './theme';
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

  app.onhostcontextchanged = (ctx: any) => {
    applyTheme(ctx?.theme ?? app.getHostContext()?.theme);
  };

  app.ontoolresult = (params: any) => {
    if (params?.isError) {
      state.value = 'error';
      message.value = errorText(params);
      return;
    }
    data.value = params?.structuredContent ?? {};
    state.value = 'ready';
  };

  try {
    applyTheme(undefined);
    await app.connect();
    applyTheme(app.getHostContext()?.theme);
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
  <component :is="view.component" v-if="state === 'ready'" :data="data" v-bind="view.descriptor ? { descriptor: view.descriptor } : {}" />
  <ViewShell v-else title="OAgent">
    <p v-if="state === 'connecting'" class="text-sm text-base-content/60">
      {{ t('mcp.connecting') }}
    </p>
    <ErrorText v-else-if="state === 'error'" :error="{ message }" />
    <p v-else class="text-sm text-base-content/60">
      {{ t('mcp.waiting') }}
    </p>
  </ViewShell>
</template>
