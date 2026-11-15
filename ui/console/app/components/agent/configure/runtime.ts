import type { SelectAllState } from '#base/components/ui/selectAll';
import type { AgentConfig, AgentToolConfig } from './agentConfig';
import type { KnowledgeSource } from '~/stores/knowledge';
import type { McpServer, McpTool } from '~/stores/mcpServers';
import type { MemoryStore } from '~/stores/memoryStores';
import type { ModelProvider, ProviderModel } from '~/stores/modelProviders';
import { selectAllState } from '#base/components/ui/selectAll';
import { toolName } from '~/components/mcpserver/tools';
import { hasPrice } from '~/components/provider/useProviderForm';
import { promptTooLong } from './agentConfig';

export type BlockerTarget = 'instructions' | 'model' | 'tools' | 'web' | 'limits';

export const webListMax = 50;
export const webHeaderMax = 20;

export interface Blocker {
  key: string;
  target: BlockerTarget;
  field?: string;
  params?: Record<string, string>;
}

export interface ConfigureLinks {
  providers: string;
  servers: string;
  webTools: string;
  knowledge: string;
  endUsers: string;
  memoryStores: string;
  channels: string;
}

export type MemoryTrust = Pick<MemoryStore, 'id' | 'rememberApiUsers'>;

export interface RuntimeContext {
  providers: ModelProvider[];
  servers: McpServer[];
  memoryStores: MemoryTrust[];
  searchReady?: boolean;
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

// readsUntrusted reports whether the agent reads documents, memory that API keys can write, or the web, so its tools need a person's approval.
export function readsUntrusted(config: AgentConfig, memoryStores: MemoryTrust[]): boolean {
  return config.knowledgeSourceIds.length > 0 || memoryStores.some(s => s.id === config.memoryStoreId && s.rememberApiUsers)
    || webOn(config);
}

// webOn reports whether any built-in web tool is on.
export function webOn(config: AgentConfig): boolean {
  return config.web.search || config.web.fetch || config.web.http;
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

// hostOf keeps only the lowercase host name of a typed host or pasted URL: no scheme, user, port, path, query or fragment.
export function hostOf(text: string): string {
  return text.trim().replace(/^[a-z][a-z0-9+.-]*:\/\//i, '').replace(/^[^/?#]*@/, '').replace(/[/?#].*$/, '').replace(/:\d*$/, '').toLowerCase();
}

// parseLines turns a textarea into trimmed, deduped entries, one per line, as host names when host is set; over says the list is longer than max.
export function parseLines(text: string, opts: { host: boolean, max: number }): { lines: string[], over: boolean } {
  const lines = [...new Set(text.split('\n').map(l => (opts.host ? hostOf(l) : l.trim())).filter(Boolean))];
  return { lines, over: lines.length > opts.max };
}

// parseSeconds reads whole seconds into milliseconds: empty means the default (0), and null means not a whole number from 1 up to maxMs.
export function parseSeconds(text: string, maxMs?: number): number | null {
  const n = parseCount(text, maxMs === undefined ? undefined : Math.floor(maxMs / 1000));
  return n === null ? null : n * 1000;
}

// parseCount reads a whole number: empty means the default (0), and null means not a whole number from 1 up to max.
export function parseCount(text: string, max?: number): number | null {
  const clean = text.trim();
  if (!clean) {
    return 0;
  }
  const n = /^\d+$/.test(clean) ? Number(clean) : 0;
  return n >= 1 && (max === undefined || n <= max) ? n : null;
}

// secretsFor counts the project HTTP secrets whose host the agent may call.
export function secretsFor(hosts: string[], secrets: { host: string }[]): number {
  return secrets.filter(s => hosts.includes(s.host)).length;
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
  if (promptTooLong(config)) {
    out.push({ key: 'publish.blocker_prompt_long', target: 'instructions' });
  }
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
  return [...out, ...webBlockers(config, ctx.searchReady)];
}

function webBlockers(config: AgentConfig, searchReady?: boolean): Blocker[] {
  const { web } = config;
  const out: Blocker[] = [];
  const tooMany = (field: string, max: number): Blocker => ({ key: 'publish.blocker_web_too_many', target: 'web', field, params: { max: String(max) } });
  if (web.search && searchReady === false) {
    out.push({ key: 'publish.blocker_web_search_provider', target: 'web', field: 'web-search' });
  }
  if (web.fetch && web.fetchAllowDomains.length > webListMax) {
    out.push(tooMany('web-domains', webListMax));
  } else if (web.fetch && web.fetchAllowDomains.length === 0) {
    out.push({ key: 'publish.blocker_web_fetch_domains', target: 'web', field: 'web-domains' });
  }
  if (web.http && web.httpHosts.length > webListMax) {
    out.push(tooMany('web-hosts', webListMax));
  } else if (web.http && web.httpHosts.length === 0) {
    out.push({ key: 'publish.blocker_web_http_hosts', target: 'web', field: 'web-hosts' });
  }
  if (web.http && web.requestHeaders.length > webHeaderMax) {
    out.push(tooMany('web-headers', webHeaderMax));
  }
  return out;
}

// publishWarnings lists what the server may refuse while the draft can still save, each pointing at its section.
export function publishWarnings(config: AgentConfig, ctx: RuntimeContext): Blocker[] {
  const tools = readsUntrusted(config, ctx.memoryStores) ? unattendedTools(config, ctx.servers) : [];
  return tools.length ? [{ key: 'publish.warning_unapproved_tool', target: 'tools', field: tools[0], params: { tool: tools[0]!, n: String(tools.length - 1) } }] : [];
}
