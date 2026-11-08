<script setup lang="ts">
import type { AgentConfig } from './agentConfig';
import type { McpServer, McpTool } from '~/stores/mcpServers';
import AppButton from '#base/components/ui/AppButton.vue';
import ErrorText from '#base/components/ui/ErrorText.vue';
import SearchInput from '#base/components/ui/SearchInput.vue';
import SelectAllCheckbox from '#base/components/ui/SelectAllCheckbox.vue';
import { hintBadges, nameParts, toolName } from '~/components/mcpserver/tools';
import { useMcpServersStore } from '~/stores/mcpServers';
import { grantFor, grantState, missingServers, withAllGrants, withApproval, withGrant, withoutGrant } from './runtime';

const { servers, disabled = false, serversLink } = defineProps<{ servers: McpServer[], disabled?: boolean, serversLink: string }>();

const config = defineModel<AgentConfig>({ required: true });
const { t } = useI18n();

const missing = computed(() => missingServers(config.value, servers));
const open = Object.fromEntries(servers.map(s => [s.id, servers.length === 1]));
const query = ref<Record<string, string>>({});
const filterFrom = 9;
const store = useMcpServersStore();
const uid = useId();

function onCount(server: McpServer) {
  return config.value.grants.find(g => g.serverId === server.id)?.tools.length ?? 0;
}

function countLabel(server: McpServer) {
  if (server.tools.length) {
    return t('grants.on_count', { on: onCount(server), total: server.tools.length });
  }
  return onCount(server) ? t('grants.on_count_unloaded', { on: onCount(server) }) : t('grants.on_count_none');
}

function descId(server: McpServer, tool: McpTool) {
  return `${uid}-${server.id}-${tool.name}`;
}

function onNames(server: McpServer) {
  const names = config.value.grants.find(g => g.serverId === server.id)?.tools.map(t => t.name) ?? [];
  return names.length > 3 ? t('grants.on_names_more', { names: names.slice(0, 3).join(', '), n: names.length - 3 }) : names.join(', ');
}

function filtering(server: McpServer) {
  return !!query.value[server.id]?.trim();
}

function shown(server: McpServer) {
  const q = query.value[server.id]?.trim().toLowerCase();
  return q ? server.tools.filter(tool => `${toolName(server.name, tool.name)} ${tool.description}`.toLowerCase().includes(q)) : server.tools;
}

function shownGrantable(server: McpServer) {
  return shown(server).filter(tool => tool.grantable);
}

function clearFilter(server: McpServer, event: Event) {
  query.value[server.id] = '';
  (event.currentTarget as HTMLElement).closest('details')?.querySelector<HTMLInputElement>('input[type="search"]')?.focus();
}

function onToggle(server: McpServer, event: Event) {
  if ((event.target as HTMLDetailsElement).open) {
    void store.ensureTools(server.id);
  }
}

onMounted(() => {
  servers.filter(s => open[s.id] || onCount(s) > 0).forEach(s => void store.ensureTools(s.id));
});

function setOn(server: McpServer, tool: McpTool, on: boolean) {
  config.value = withGrant(config.value, server, tool, on);
}

function setAll(server: McpServer, on: boolean) {
  config.value = withAllGrants(config.value, server, shown(server), on);
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
        <span class="sr-only">{{ server.tools.length ? t('grants.collapse_label', { server: server.name, on: onCount(server), total: server.tools.length }) : t('grants.collapse_label_unloaded', { server: server.name, on: onCount(server) }) }}</span>
        <span class="font-mono text-xs font-medium" aria-hidden="true">{{ server.name }}</span>
        <span v-if="server.trusted" class="badge badge-sm badge-soft text-base-content"><Icon name="lucide:shield-check" size="12" aria-hidden="true" />{{ t('mcpservers.trusted_badge') }}</span>
        <span v-else class="badge badge-sm badge-outline">{{ t('mcpservers.untrusted_badge') }}</span>
        <span class="text-xs text-base-content/70" aria-hidden="true">{{ countLabel(server) }}</span>
        <span v-if="onCount(server)" class="min-w-0 basis-full font-mono text-xs break-words text-base-content/70 sm:basis-auto" aria-hidden="true" data-testid="on-names">{{ onNames(server) }}</span>
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
        <div v-if="server.tools.length >= filterFrom" class="flex flex-col border-t border-base-300 px-4 py-2">
          <SearchInput v-model="query[server.id]" class="w-full" :label="t('grants.filter_label', { server: server.name })" :placeholder="t('grants.filter_placeholder')" />
          <p role="status" class="mt-1 text-xs text-base-content/70 empty:mt-0">
            {{ filtering(server) ? t('grants.filter_count', { n: shown(server).length, total: server.tools.length }) : '' }}
          </p>
        </div>
        <p v-if="server.tools.length && !shown(server).length" class="flex flex-wrap items-center gap-2 border-t border-base-300 px-4 py-2 text-ui text-base-content/70">
          {{ t('grants.filter_none', { query: query[server.id]?.trim() }) }}
          <AppButton variant="plain" size="xs" @click="clearFilter(server, $event)">
            {{ t('grants.filter_clear') }}
          </AppButton>
        </p>
        <SelectAllCheckbox
          v-if="shownGrantable(server).length" class="border-t border-base-300 px-4 py-2" data-testid="select-all"
          :state="grantState(config, server, shown(server))" :label="t(filtering(server) ? 'grants.select_all_shown' : 'grants.select_all', { n: shownGrantable(server).length })"
          :context="t('grants.select_all_context', { server: server.name })" :disabled="disabled" @change="setAll(server, $event)"
        />
        <ul v-if="shown(server).length" class="divide-y divide-base-300 border-t border-base-300">
          <li v-for="tool in shown(server)" :key="tool.name" class="flex flex-col gap-2 px-4 py-2 text-ui">
            <div class="flex flex-wrap items-start gap-x-3 gap-y-1">
              <label v-if="tool.grantable" class="flex min-w-0 flex-1 basis-56 items-start gap-2">
                <input
                  type="checkbox" class="checkbox checkbox-sm mt-0.5 rounded-sm" :checked="!!grantFor(config, server.id, tool.name)" :disabled="disabled" :aria-describedby="tool.description ? descId(server, tool) : undefined"
                  @change="setOn(server, tool, ($event.target as HTMLInputElement).checked)"
                >
                <span class="min-w-0">
                  <span class="sr-only">{{ t('grants.use_tool', { tool: toolName(server.name, tool.name) }) }}</span>
                  <span class="block font-mono text-xs" aria-hidden="true"><template v-for="(part, i) in nameParts(toolName(server.name, tool.name))" :key="i">{{ part }}<wbr></template></span>
                  <span :id="descId(server, tool)" class="line-clamp-1 text-xs text-base-content/70" aria-hidden="true">{{ tool.description }}</span>
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
