<script setup lang="ts">
import type { McpServer } from '~/stores/mcpServers';
import AppButton from '#base/components/ui/AppButton.vue';
import ErrorText from '#base/components/ui/ErrorText.vue';
import { useMcpServersStore } from '~/stores/mcpServers';
import McpToolList from './McpToolList.vue';

const { server, trusted } = defineProps<{ server: McpServer, trusted: boolean }>();
const emit = defineEmits<{ replaceSecret: [] }>();

const { t } = useI18n();
const servers = useMcpServersStore();
const busy = computed(() => !!servers.toolsLoading[server.id]);
const error = computed(() => servers.toolsError[server.id] ?? null);
</script>

<template>
  <section class="flex flex-col gap-2" :aria-label="t('mcpservers.tools')">
    <div class="flex flex-wrap items-center justify-between gap-2">
      <h3 class="text-ui font-semibold">
        {{ t('mcpservers.tools') }}
      </h3>
      <AppButton variant="outline" size="sm" :busy="busy" @click="servers.loadTools(server.id)">
        <Icon v-if="!busy" name="lucide:refresh-cw" size="14" aria-hidden="true" />
        {{ t('mcpservers.load_tools') }}
      </AppButton>
    </div>
    <div aria-live="polite" class="flex flex-col gap-2">
      <p v-if="busy" role="status" class="flex items-center gap-2 text-ui text-base-content/70">
        <span class="loading loading-spinner loading-xs" aria-hidden="true" />
        {{ t('mcpservers.probing', { name: server.name }) }}
      </p>
      <div v-else-if="servers.toolsRefused[server.id]" role="alert" class="alert alert-error alert-soft alert-vertical text-base-content text-ui sm:alert-horizontal">
        <Icon name="lucide:key-round" size="16" class="shrink-0" aria-hidden="true" />
        <span>{{ t('mcpservers.secret_refused') }}</span>
        <AppButton variant="plain" size="xs" @click="emit('replaceSecret')">
          {{ t('mcpservers.replace_secret') }}
        </AppButton>
      </div>
      <ErrorText v-else-if="error" :error="error" />
      <template v-else-if="server.tools.length">
        <p class="text-xs text-base-content/70">
          {{ t('mcpservers.tool_count', { n: server.tools.length }, server.tools.length) }}
        </p>
        <McpToolList :server="server.name" :tools="server.tools" :trusted="trusted" />
      </template>
      <p v-else class="text-xs text-base-content/70">
        {{ t('mcpservers.tools_hint') }}
      </p>
    </div>
  </section>
</template>
