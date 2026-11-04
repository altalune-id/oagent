import type { Change } from './agentConfig';
import type { AgentRuntimeConfig, AskWhen, ToolGrant } from '~/stores/agentRuntime';
import type { KnowledgeSource } from '~/stores/knowledge';
import type { McpServer, McpTool } from '~/stores/mcpServers';
import type { ModelProvider } from '~/stores/providers';
import { hasPrice } from '~/components/provider/useProviderForm';

export type BlockerTarget = 'model' | 'tools' | 'web' | 'limits';

export interface Blocker {
  key: string;
  target: BlockerTarget;
  params?: Record<string, string>;
}

export interface ConfigureLinks {
  providers: string;
  servers: string;
  webSearch: string;
  knowledge: string;
}

export interface RuntimeContext {
  providers: ModelProvider[];
  servers: McpServer[];
  maxCostCents: number;
  oldTools: number;
}

// defaultAsk picks the approval default for a tool turned on: a trusted server's read-only hint runs freely, everything else asks.
export function defaultAsk(server: McpServer, tool: McpTool): AskWhen {
  return server.trusted && tool.hint === 'read_only' ? 'never' : 'always';
}

// booleanArgs lists the tool arguments an approval condition can match on.
export function booleanArgs(tool: McpTool): string[] {
  return tool.args.filter(a => a.type === 'boolean').map(a => a.name);
}

// grantFor returns the grant of one server tool, if the agent has it.
export function grantFor(config: AgentRuntimeConfig, serverId: string, tool: string): ToolGrant | undefined {
  return config.grants.find(g => g.serverId === serverId && g.tool === tool);
}

// withGrant turns one tool on (with its default approval) or off.
export function withGrant(config: AgentRuntimeConfig, server: McpServer, tool: McpTool, on: boolean): AgentRuntimeConfig {
  const rest = config.grants.filter(g => !(g.serverId === server.id && g.tool === tool.name));
  const grants = on ? [...rest, { serverId: server.id, serverName: server.name, tool: tool.name, ask: defaultAsk(server, tool) }] : rest;
  return { ...config, grants };
}

// withAsk changes when one granted tool asks; a match rule starts on the first boolean argument being true.
export function withAsk(config: AgentRuntimeConfig, serverId: string, tool: McpTool, ask: AskWhen): AgentRuntimeConfig {
  const grants = config.grants.map((g) => {
    if (g.serverId !== serverId || g.tool !== tool.name) {
      return g;
    }
    return ask === 'match'
      ? { ...g, ask, matchArg: g.matchArg ?? booleanArgs(tool)[0], matchValue: g.matchValue ?? true }
      : { serverId: g.serverId, serverName: g.serverName, tool: g.tool, ask };
  });
  return { ...config, grants };
}

// withMatch sets the argument and value a match rule asks on.
export function withMatch(config: AgentRuntimeConfig, serverId: string, tool: string, arg: string, value: boolean): AgentRuntimeConfig {
  return { ...config, grants: config.grants.map(g => (g.serverId === serverId && g.tool === tool ? { ...g, matchArg: arg, matchValue: value } : g)) };
}

// missingServers lists servers the grants still name but the project no longer has.
export function missingServers(config: AgentRuntimeConfig, servers: McpServer[]): { id: string, name: string, tools: number }[] {
  const known = new Set(servers.map(s => s.id));
  const out = new Map<string, { id: string, name: string, tools: number }>();
  for (const g of config.grants.filter(g => !known.has(g.serverId))) {
    const entry = out.get(g.serverId) ?? { id: g.serverId, name: g.serverName, tools: 0 };
    entry.tools++;
    out.set(g.serverId, entry);
  }
  return [...out.values()];
}

// readsPrivateData reports whether the agent can see private data, so web_fetch needs an allow-list; any granted MCP tool counts.
export function readsPrivateData(config: AgentRuntimeConfig): boolean {
  return config.grants.length > 0 || config.memory || config.knowledgeIds.length > 0;
}

// parseDollars reads a typed dollar amount into cents: empty means no cap (0), and null means the text is not an amount.
export function parseDollars(text: string): number | null {
  const clean = text.trim().replace(/^\$/, '').replace(',', '.');
  if (!clean) {
    return 0;
  }
  return /^\d+(?:\.\d{0,2})?$/.test(clean) ? Math.round(Number(clean) * 100) : null;
}

// parseDomains turns the allow-list textarea into host names, one per line.
export function parseDomains(text: string): string[] {
  return text.split('\n').map(d => d.trim().toLowerCase()).filter(Boolean);
}

// embeddingReady reports whether any provider has a model marked for embeddings, which knowledge needs.
export function embeddingReady(providers: ModelProvider[]): boolean {
  return providers.some(p => p.models.some(m => m.embedding));
}

// readySources keeps the knowledge sources an agent may use now.
export function readySources(sources: KnowledgeSource[]): KnowledgeSource[] {
  return sources.filter(s => s.status === 'ready');
}

// chosenModel returns the provider and model the draft names, when both still exist.
export function chosenModel(config: AgentRuntimeConfig, providers: ModelProvider[]) {
  const provider = providers.find(p => p.id === config.providerId);
  return { provider, model: provider?.models.find(m => m.id === config.model) };
}

// costCapError returns the text key for a cost cap the chosen model can't honour, or ''.
export function costCapError(config: AgentRuntimeConfig, ctx: Pick<RuntimeContext, 'providers' | 'maxCostCents'>): string {
  const { model } = chosenModel(config, ctx.providers);
  return ctx.maxCostCents > 0 && model && !hasPrice(model) ? 'limits.unpriced' : '';
}

// publishBlockers lists what must be fixed before this draft may go live, each pointing at its section.
export function publishBlockers(config: AgentRuntimeConfig, ctx: RuntimeContext): Blocker[] {
  const out: Blocker[] = [];
  const { provider, model } = chosenModel(config, ctx.providers);
  if (!provider || !config.model) {
    out.push({ key: 'publish.blocker_model', target: 'model' });
  } else if (!model) {
    out.push({ key: 'publish.blocker_model_removed', target: 'model', params: { model: config.model, provider: provider.name } });
  }
  if (costCapError(config, ctx)) {
    out.push({ key: 'publish.blocker_unpriced', target: 'limits', params: { provider: provider?.name ?? '' } });
  }
  if (config.webFetch && readsPrivateData(config) && config.allowedDomains.length === 0) {
    out.push({ key: 'publish.blocker_allowlist', target: 'web' });
  }
  for (const s of missingServers(config, ctx.servers)) {
    out.push({ key: 'publish.blocker_server_missing', target: 'tools', params: { server: s.name } });
  }
  if (ctx.oldTools > 0) {
    out.push({ key: 'publish.blocker_legacy_tools', target: 'tools' });
  }
  return out;
}

function askLabel(g: ToolGrant): string {
  return g.ask === 'match' ? `${g.ask}:${g.matchArg}=${g.matchValue}` : g.ask;
}

// runtimeChanges lists what publishing changes in the runtime config, as locale keys; a legacy live version has none.
export function runtimeChanges(draft: AgentRuntimeConfig, live: AgentRuntimeConfig | undefined, providers: ModelProvider[]): Change[] {
  const out: Change[] = [];
  const name = (id: string) => providers.find(p => p.id === id)?.name ?? '—';
  if (draft.providerId !== live?.providerId || draft.model !== live?.model) {
    out.push({ key: 'configure.change_provider_model', params: { provider: name(draft.providerId), model: draft.model || '—' } });
  }
  if (draft.helperModel !== (live?.helperModel ?? '')) {
    out.push({ key: 'configure.change_helper', params: { model: draft.helperModel || '—' } });
  }
  const before = new Map((live?.grants ?? []).map(g => [`${g.serverName}__${g.tool}`, g]));
  for (const g of draft.grants) {
    const id = `${g.serverName}__${g.tool}`;
    const old = before.get(id);
    if (!old) {
      out.push({ key: g.ask === 'never' ? 'configure.change_tool_added' : 'configure.change_tool_added_approval', params: { tool: id } });
    } else if (askLabel(old) !== askLabel(g)) {
      out.push({ key: 'configure.change_tool_ask', params: { tool: id } });
    }
    before.delete(id);
  }
  for (const id of before.keys()) {
    out.push({ key: 'configure.change_tool_removed', params: { tool: id } });
  }
  const fetchChanged = draft.webFetch !== (live?.webFetch ?? false) || draft.allowedDomains.join() !== (live?.allowedDomains ?? []).join();
  if (fetchChanged && !draft.webFetch) {
    out.push({ key: 'configure.change_web_fetch_off' });
  } else if (fetchChanged && draft.allowedDomains.length) {
    out.push({ key: 'configure.change_web_fetch_on', params: { domains: draft.allowedDomains.join(', ') } });
  }
  if (draft.webSearch !== (live?.webSearch ?? false)) {
    out.push({ key: draft.webSearch ? 'configure.change_web_search_on' : 'configure.change_web_search_off' });
  }
  if (draft.memory !== (live?.memory ?? false)) {
    out.push({ key: draft.memory ? 'configure.change_memory_on' : 'configure.change_memory_off' });
  }
  if (draft.knowledgeIds.join() !== (live?.knowledgeIds ?? []).join()) {
    out.push({ key: 'configure.change_knowledge', params: { n: String(draft.knowledgeIds.length) } });
  }
  return out;
}
