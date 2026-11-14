import type { SelectAllState } from '#base/components/ui/selectAll';
import type { AgentConfig, AgentToolConfig } from './agentConfig';
import type { KnowledgeSource } from '~/stores/knowledge';
import type { McpServer, McpTool } from '~/stores/mcpServers';
import type { ModelProvider, ProviderModel } from '~/stores/modelProviders';
import { selectAllState } from '#base/components/ui/selectAll';
import { toolName } from '~/components/mcpserver/tools';
import { hasPrice } from '~/components/provider/useProviderForm';

export type BlockerTarget = 'model' | 'tools' | 'limits';

export interface Blocker {
  key: string;
  target: BlockerTarget;
  field?: string;
  params?: Record<string, string>;
}

export interface ConfigureLinks {
  providers: string;
  servers: string;
  webSearch: string;
  knowledge: string;
  endUsers: string;
  memoryStores: string;
  channels: string;
}

export interface RuntimeContext {
  providers: ModelProvider[];
  servers: McpServer[];
}

export interface MissingServer {
  id: string;
  tools: number;
}

// grantFor returns the grant of one server tool, if the agent has it.
export function grantFor(config: AgentConfig, serverId: string, tool: string): AgentToolConfig | undefined {
  return config.grants.find(g => g.serverId === serverId)?.tools.find(t => t.name === tool);
}

// withGrant turns one tool on, asking for approval as the server suggests, or off; a server left with no tools is dropped.
export function withGrant(config: AgentConfig, server: McpServer, tool: McpTool, on: boolean): AgentConfig {
  const grant = config.grants.find(g => g.serverId === server.id) ?? { serverId: server.id, tools: [], missing: false };
  const rest = grant.tools.filter(t => t.name !== tool.name);
  const tools = on ? [...rest, { name: tool.name, requireApproval: tool.requireApproval }] : rest;
  const others = config.grants.filter(g => g.serverId !== server.id);
  return { ...config, grants: tools.length ? [...others, { ...grant, tools }] : others };
}

// grantState says whether all, some or none of the given grantable tools of a server are on.
export function grantState(config: AgentConfig, server: McpServer, tools: McpTool[] = server.tools): SelectAllState {
  const grantable = tools.filter(t => t.grantable);
  return selectAllState(grantable.filter(t => grantFor(config, server.id, t.name)).length, grantable.length);
}

// withAllGrants turns the given grantable tools of a server on, keeping the approval of tools already on, or turns them off.
export function withAllGrants(config: AgentConfig, server: McpServer, tools: McpTool[], on: boolean): AgentConfig {
  return tools.filter(t => t.grantable && !grantFor(config, server.id, t.name) === on).reduce((c, t) => withGrant(c, server, t, on), config);
}

// withApproval sets whether one granted tool needs a person's approval.
// withToolConfig replaces one granted tool's settings, such as its approval and free_when rule.
export function withToolConfig(config: AgentConfig, serverId: string, next: AgentToolConfig): AgentConfig {
  return { ...config, grants: config.grants.map(g => (g.serverId === serverId ? { ...g, tools: g.tools.map(t => (t.name === next.name ? next : t)) } : g)) };
}

export function withApproval(config: AgentConfig, serverId: string, tool: string, requireApproval: boolean): AgentConfig {
  return {
    ...config,
    grants: config.grants.map(g => (g.serverId === serverId ? { ...g, tools: g.tools.map(t => (t.name === tool ? { ...t, requireApproval } : t)) } : g)),
  };
}

// missingServers lists grants whose server no longer exists, as the server marks them or the project no longer lists them.
export function missingServers(config: AgentConfig, servers: McpServer[]): MissingServer[] {
  const known = new Set(servers.map(s => s.id));
  return config.grants.filter(g => g.missing || !known.has(g.serverId)).map(g => ({ id: g.serverId, tools: g.tools.length }));
}

// withoutGrant drops every tool of one server.
export function withoutGrant(config: AgentConfig, serverId: string): AgentConfig {
  return { ...config, grants: config.grants.filter(g => g.serverId !== serverId) };
}

// readsPrivateData reports whether the agent can see private data, so web_fetch needs an allow-list; any granted MCP tool counts.
export function readsPrivateData(config: AgentConfig): boolean {
  return config.grants.length > 0 || !!config.memoryStoreId || config.knowledgeSourceIds.length > 0;
}

// readsUntrusted reports whether the agent reads text a third party wrote, so its tools need a person's approval.
export function readsUntrusted(config: AgentConfig): boolean {
  return config.knowledgeSourceIds.length > 0 || !!config.memoryStoreId;
}

// unattendedTools lists the granted tools that run without approval, as the model names them.
export function unattendedTools(config: AgentConfig, servers: McpServer[]): string[] {
  return config.grants.flatMap((g) => {
    const server = servers.find(s => s.id === g.serverId);
    return g.missing || !server ? [] : g.tools.filter(t => !t.requireApproval).map(t => toolName(server.name, t.name));
  });
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

// bindable reports whether an agent can search a knowledge source: it has a ready document.
export function bindable(source: KnowledgeSource): boolean {
  return source.ready > 0;
}

// chosenModel returns the provider and the model and helper model the draft names, when they still exist.
export function chosenModel(config: Pick<AgentConfig, 'providerId' | 'model'> & Partial<Pick<AgentConfig, 'helperModel'>>, providers: ModelProvider[]): { provider?: ModelProvider, model?: ProviderModel, helper?: ProviderModel } {
  const provider = providers.find(p => p.id === config.providerId);
  return { provider, model: provider?.models.find(m => m.id === config.model), helper: config.helperModel ? provider?.models.find(m => m.id === config.helperModel) : undefined };
}

// costCapError returns the text key for a cost cap the chosen model can't honour, or ''.
export function costCapError(config: AgentConfig, providers: ModelProvider[]): string {
  const { model } = chosenModel(config, providers);
  return config.maxCostCents > 0 && model && !hasPrice(model) ? 'limits.unpriced' : '';
}

// helperCapError returns the text key for a cost cap the chosen helper model can't honour, or ''.
export function helperCapError(config: AgentConfig, providers: ModelProvider[]): string {
  const { helper } = chosenModel(config, providers);
  return config.maxCostCents > 0 && helper && !hasPrice(helper) ? 'limits.helper_unpriced' : '';
}

// publishBlockers lists what must be fixed before this draft may go live, each pointing at its section.
export function publishBlockers(config: AgentConfig, ctx: RuntimeContext): Blocker[] {
  const out: Blocker[] = [];
  const { provider, model } = chosenModel(config, ctx.providers);
  if (!provider || !config.model) {
    out.push({ key: config.providerMissing ? 'publish.blocker_provider_missing' : 'publish.blocker_model', target: 'model' });
  } else if (!model) {
    out.push({ key: 'publish.blocker_model_removed', target: 'model', params: { model: config.model, provider: provider.name } });
  }
  if (costCapError(config, ctx.providers)) {
    out.push({ key: 'publish.blocker_unpriced', target: 'limits', params: { provider: provider?.name ?? '' } });
  }
  if (helperCapError(config, ctx.providers)) {
    out.push({ key: 'publish.blocker_helper_unpriced', target: 'model', field: 'helper', params: { model: config.helperModel, provider: provider?.name ?? '' } });
  }
  if (missingServers(config, ctx.servers).length) {
    out.push({ key: 'publish.blocker_server_missing', target: 'tools' });
  }
  if (config.tools.length > 0) {
    out.push({ key: 'publish.blocker_legacy_tools', target: 'tools' });
  }
  return out;
}

// publishWarnings lists what the server may refuse while the draft can still save, each pointing at its section.
export function publishWarnings(config: AgentConfig, ctx: RuntimeContext): Blocker[] {
  const tools = readsUntrusted(config) ? unattendedTools(config, ctx.servers) : [];
  return tools.length ? [{ key: 'publish.warning_unapproved_tool', target: 'tools', field: tools[0], params: { tool: tools[0]!, n: String(tools.length - 1) } }] : [];
}
