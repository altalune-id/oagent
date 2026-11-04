<script setup lang="ts">
import type { AgentRuntimeConfig, AskWhen } from '~/stores/agentRuntime';
import type { McpServer, McpTool } from '~/stores/mcpServers';
import AppButton from '#base/components/ui/AppButton.vue';
import StatusLabel from '#base/components/ui/StatusLabel.vue';
import { grantable, hintBadges, nameParts, toolName } from '~/components/mcpserver/tools';
import { booleanArgs, grantFor, missingServers, withAsk, withGrant, withMatch } from './runtime';

const { servers, disabled = false, serversLink } = defineProps<{ servers: McpServer[], disabled?: boolean, serversLink: string }>();

const runtime = defineModel<AgentRuntimeConfig>({ required: true });
const { t } = useI18n();
const uid = useId();

const missing = computed(() => missingServers(runtime.value, servers));
const wide = typeof window !== 'undefined' && !!window.matchMedia?.('(min-width: 40rem)').matches;
const open = Object.fromEntries(servers.map(s => [s.id, wide && runtime.value.grants.some(g => g.serverId === s.id)]));

function onCount(server: McpServer) {
  return runtime.value.grants.filter(g => g.serverId === server.id).length;
}

function setOn(server: McpServer, tool: McpTool, on: boolean) {
  runtime.value = withGrant(runtime.value, server, tool, on);
}

function setAsk(server: McpServer, tool: McpTool, ask: AskWhen) {
  runtime.value = withAsk(runtime.value, server.id, tool, ask);
}

function setMatch(server: McpServer, tool: string, arg: string, value: boolean) {
  runtime.value = withMatch(runtime.value, server.id, tool, arg, value);
}

function removeMissing(serverId: string) {
  runtime.value = { ...runtime.value, grants: runtime.value.grants.filter(g => g.serverId !== serverId) };
}

function rowId(server: McpServer, tool: McpTool) {
  return `${uid}-${server.name}-${tool.name}`;
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

    <div v-for="m in missing" :key="m.id" class="flex flex-wrap items-center justify-between gap-2 rounded-box border border-base-300 px-4 py-3">
      <span class="flex flex-wrap items-center gap-2">
        <span class="font-mono text-xs">{{ m.name }}</span>
        <StatusLabel tone="error" :label="t('grants.server_missing')" :detail="t('grants.tool_count', { n: m.tools }, m.tools)" />
      </span>
      <AppButton variant="plain" size="xs" :disabled="disabled" @click="removeMissing(m.id)">
        {{ t('grants.remove_missing') }}
      </AppButton>
    </div>

    <details v-for="server in servers" :key="server.id" class="collapse-arrow collapse rounded-box border border-base-300 bg-base-100" :open="open[server.id]">
      <summary class="collapse-title flex flex-wrap items-center gap-2 py-3 text-ui">
        <span class="sr-only">{{ t('grants.collapse_label', { server: server.name, on: onCount(server), total: server.tools.length }) }}</span>
        <span class="font-mono text-xs font-medium" aria-hidden="true">{{ server.name }}</span>
        <span v-if="server.trusted" class="badge badge-sm badge-soft text-base-content"><Icon name="lucide:shield-check" size="12" aria-hidden="true" />{{ t('mcpservers.trusted_badge') }}</span>
        <span v-else class="badge badge-sm badge-outline">{{ t('mcpservers.untrusted_badge') }}</span>
        <span class="text-xs text-base-content/70" aria-hidden="true">{{ t('grants.on_count', { on: onCount(server), total: server.tools.length }) }}</span>
      </summary>
      <div class="collapse-content px-0 pb-0">
        <ul class="divide-y divide-base-300 border-t border-base-300">
          <li v-for="tool in server.tools" :key="tool.name" class="flex flex-col gap-2 px-4 py-2 text-ui">
            <div class="flex flex-wrap items-start gap-x-3 gap-y-1">
              <label v-if="grantable(server.name, tool)" class="flex min-w-0 flex-1 basis-56 items-start gap-2">
                <input
                  type="checkbox" class="checkbox checkbox-sm mt-0.5 rounded-sm" :checked="!!grantFor(runtime, server.id, tool.name)" :disabled="disabled"
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

            <div v-if="grantFor(runtime, server.id, tool.name)" class="flex flex-col gap-2 ps-7">
              <div class="flex flex-wrap items-center gap-x-4 gap-y-2">
                <label class="label cursor-pointer gap-2 text-ui text-base-content">
                  <input
                    type="checkbox" class="toggle toggle-sm" :checked="grantFor(runtime, server.id, tool.name)!.ask !== 'never'" :disabled="disabled"
                    @change="setAsk(server, tool, ($event.target as HTMLInputElement).checked ? 'always' : 'never')"
                  >
                  {{ t('approvalrule.ask') }}
                </label>
                <label v-if="grantFor(runtime, server.id, tool.name)!.ask !== 'never'" class="flex items-center gap-2 text-ui">
                  <span>{{ t('approvalrule.when') }}</span>
                  <select
                    class="select select-sm w-auto text-ui" :value="grantFor(runtime, server.id, tool.name)!.ask" :disabled="disabled"
                    :aria-describedby="grantFor(runtime, server.id, tool.name)!.ask === 'match' ? `${rowId(server, tool)}-rule` : undefined"
                    @change="setAsk(server, tool, ($event.target as HTMLSelectElement).value as AskWhen)"
                  >
                    <option value="always">
                      {{ t('approvalrule.every_call') }}
                    </option>
                    <option value="match" :disabled="!booleanArgs(tool).length">
                      {{ t('approvalrule.only_match') }}
                    </option>
                  </select>
                </label>
              </div>
              <div v-if="grantFor(runtime, server.id, tool.name)!.ask === 'match'" class="flex flex-wrap items-center gap-2">
                <label class="flex items-center gap-2 text-ui">
                  <span>{{ t('approvalrule.argument') }}</span>
                  <select
                    class="select select-sm w-auto font-mono text-xs" :value="grantFor(runtime, server.id, tool.name)!.matchArg" :disabled="disabled" :aria-describedby="`${rowId(server, tool)}-rule`"
                    @change="setMatch(server, tool.name, ($event.target as HTMLSelectElement).value, grantFor(runtime, server.id, tool.name)!.matchValue ?? true)"
                  >
                    <option v-for="arg in booleanArgs(tool)" :key="arg" :value="arg">
                      {{ arg }}
                    </option>
                  </select>
                </label>
                <label class="flex items-center gap-2 text-ui">
                  <span>{{ t('approvalrule.value') }}</span>
                  <select
                    class="select select-sm w-auto text-ui" :value="String(grantFor(runtime, server.id, tool.name)!.matchValue)" :disabled="disabled" :aria-describedby="`${rowId(server, tool)}-rule`"
                    @change="setMatch(server, tool.name, grantFor(runtime, server.id, tool.name)!.matchArg ?? '', ($event.target as HTMLSelectElement).value === 'true')"
                  >
                    <option value="true">
                      {{ t('approvalrule.is_true') }}
                    </option>
                    <option value="false">
                      {{ t('approvalrule.is_false') }}
                    </option>
                  </select>
                </label>
                <p :id="`${rowId(server, tool)}-rule`" class="basis-full text-xs text-base-content/70">
                  {{ t('approvalrule.sentence', { arg: grantFor(runtime, server.id, tool.name)!.matchArg ?? '', safe: String(!grantFor(runtime, server.id, tool.name)!.matchValue) }) }}
                </p>
              </div>
              <div v-if="!server.trusted && grantFor(runtime, server.id, tool.name)!.ask === 'never'" role="alert" class="alert alert-warning alert-soft text-base-content text-ui">
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
