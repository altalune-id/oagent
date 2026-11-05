<script setup lang="ts">
import type { AgentConfig } from './agentConfig';
import type { McpServer, McpTool } from '~/stores/mcpServers';
import AppButton from '#base/components/ui/AppButton.vue';
import ErrorText from '#base/components/ui/ErrorText.vue';
import { hintBadges, nameParts, toolName } from '~/components/mcpserver/tools';
import { useMcpServersStore } from '~/stores/mcpServers';
import { grantFor, missingServers, withApproval, withGrant, withoutGrant } from './runtime';

const { servers, disabled = false, serversLink } = defineProps<{ servers: McpServer[], disabled?: boolean, serversLink: string }>();

const config = defineModel<AgentConfig>({ required: true });
const { t } = useI18n();

const missing = computed(() => missingServers(config.value, servers));
const wide = typeof window !== 'undefined' && !!window.matchMedia?.('(min-width: 40rem)').matches;
const open = Object.fromEntries(servers.map(s => [s.id, wide && onCount(s) > 0]));
const store = useMcpServersStore();

function onCount(server: McpServer) {
  return config.value.grants.find(g => g.serverId === server.id)?.tools.length ?? 0;
}

function onToggle(server: McpServer, event: Event) {
  if ((event.target as HTMLDetailsElement).open) {
    void store.ensureTools(server.id);
  }
}

onMounted(() => {
  servers.filter(s => onCount(s) > 0).forEach(s => void store.ensureTools(s.id));
});

function setOn(server: McpServer, tool: McpTool, on: boolean) {
  config.value = withGrant(config.value, server, tool, on);
}

function setApproval(server: McpServer, tool: McpTool, on: boolean) {
  config.value = withApproval(config.value, server.id, tool.name, on);
}

function removeMissing(serverId: string) {
  config.value = withoutGrant(config.value, serverId);
}
</script>

<template>
  <div class="flex flex-col gap-2">
    <p v-if="!servers.length && !missing.length" class="text-ui text-base-content/70">
      {{ t('grants.no_servers') }}
      <NuxtLink :to="serversLink" class="link">
        {{ t('grants.add_server') }}
      </NuxtLink>
    </p>

    <div v-for="m in missing" :key="m.id" role="alert" class="alert alert-warning alert-soft alert-vertical text-base-content text-ui sm:alert-horizontal">
      <Icon name="lucide:triangle-alert" size="16" class="shrink-0" aria-hidden="true" />
      <span>
        {{ t('grants.server_gone', { n: m.tools }, m.tools) }}
        <NuxtLink :to="serversLink" class="link">{{ t('grants.open_servers') }}</NuxtLink>
      </span>
      <AppButton variant="plain" size="xs" :disabled="disabled" @click="removeMissing(m.id)">
        {{ t('grants.remove_missing') }}
      </AppButton>
    </div>

    <details v-for="server in servers" :key="server.id" class="collapse-arrow collapse rounded-box border border-base-300 bg-base-100" :open="open[server.id]" @toggle="onToggle(server, $event)">
      <summary class="collapse-title flex flex-wrap items-center gap-2 py-3 text-ui">
        <span class="sr-only">{{ t('grants.collapse_label', { server: server.name, on: onCount(server), total: server.tools.length }) }}</span>
        <span class="font-mono text-xs font-medium" aria-hidden="true">{{ server.name }}</span>
        <span v-if="server.trusted" class="badge badge-sm badge-soft text-base-content"><Icon name="lucide:shield-check" size="12" aria-hidden="true" />{{ t('mcpservers.trusted_badge') }}</span>
        <span v-else class="badge badge-sm badge-outline">{{ t('mcpservers.untrusted_badge') }}</span>
        <span class="text-xs text-base-content/70" aria-hidden="true">{{ t('grants.on_count', { on: onCount(server), total: server.tools.length }) }}</span>
      </summary>
      <div class="collapse-content px-0 pb-0">
        <p v-if="store.toolsLoading[server.id]" role="status" class="flex items-center gap-2 px-4 py-2 text-ui text-base-content/70">
          <span class="loading loading-spinner loading-xs" aria-hidden="true" />
          {{ t('mcpservers.probing', { name: server.name }) }}
        </p>
        <p v-else-if="store.toolsRefused[server.id]" role="alert" class="px-4 py-2 text-ui">
          {{ t('mcpservers.secret_refused') }}
          <NuxtLink :to="serversLink" class="link">
            {{ t('mcpservers.replace_secret') }}
          </NuxtLink>
        </p>
        <ErrorText v-else-if="store.toolsError[server.id]" class="px-4 py-2" :error="store.toolsError[server.id] ?? null" />
        <ul class="divide-y divide-base-300 border-t border-base-300">
          <li v-for="tool in server.tools" :key="tool.name" class="flex flex-col gap-2 px-4 py-2 text-ui">
            <div class="flex flex-wrap items-start gap-x-3 gap-y-1">
              <label v-if="tool.grantable" class="flex min-w-0 flex-1 basis-56 items-start gap-2">
                <input
                  type="checkbox" class="checkbox checkbox-sm mt-0.5 rounded-sm" :checked="!!grantFor(config, server.id, tool.name)" :disabled="disabled"
                  @change="setOn(server, tool, ($event.target as HTMLInputElement).checked)"
                >
                <span class="min-w-0">
                  <span class="sr-only">{{ t('grants.use_tool', { tool: toolName(server.name, tool.name) }) }}</span>
                  <span class="block font-mono text-xs" aria-hidden="true"><template v-for="(part, i) in nameParts(toolName(server.name, tool.name))" :key="i">{{ part }}<wbr></template></span>
                  <span class="block text-xs text-base-content/70">{{ tool.description }}</span>
                </span>
              </label>
              <div v-else class="min-w-0 flex-1 basis-56 ps-7 text-base-content/70">
                <span class="block font-mono text-xs"><template v-for="(part, i) in nameParts(toolName(server.name, tool.name))" :key="i">{{ part }}<wbr></template></span>
                <span class="flex items-start gap-1.5 text-xs">
                  <Icon name="lucide:ban" size="12" class="mt-0.5 shrink-0" aria-hidden="true" />
                  {{ t('mcptools.not_grantable') }}
                </span>
              </div>
              <span v-if="tool.hint === 'destructive' && server.trusted" class="badge badge-sm badge-warning badge-soft text-base-content shrink-0">
                <Icon :name="hintBadges[tool.hint].icon" size="12" aria-hidden="true" />{{ t(hintBadges[tool.hint].labelKey) }}
              </span>
              <span v-else class="badge badge-sm badge-ghost shrink-0">
                <Icon :name="hintBadges[tool.hint].icon" size="12" aria-hidden="true" />{{ t(hintBadges[tool.hint].labelKey) }}<span v-if="!server.trusted">{{ t('mcptools.not_verified') }}</span>
              </span>
            </div>

            <div v-if="grantFor(config, server.id, tool.name)" class="flex flex-col gap-2 ps-7">
              <label class="label cursor-pointer gap-2 text-ui text-base-content">
                <input
                  type="checkbox" class="toggle toggle-sm" :checked="grantFor(config, server.id, tool.name)!.requireApproval" :disabled="disabled"
                  @change="setApproval(server, tool, ($event.target as HTMLInputElement).checked)"
                >
                {{ t('approvalrule.ask') }}
              </label>
              <div v-if="!server.trusted && !grantFor(config, server.id, tool.name)!.requireApproval" role="alert" class="alert alert-warning alert-soft text-base-content text-ui">
                <Icon name="lucide:triangle-alert" size="16" class="shrink-0" aria-hidden="true" />
                <span>{{ t('grants.untrusted_no_ask') }}</span>
              </div>
            </div>
          </li>
        </ul>
      </div>
    </details>
  </div>
</template>
