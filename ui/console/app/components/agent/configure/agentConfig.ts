import type { Timestamp } from '@bufbuild/protobuf/wkt';
import { timestampDate } from '@bufbuild/protobuf/wkt';

export interface FreeCondition {
  arg: string;
  values: string[];
  absent: boolean;
}

export interface AgentToolConfig {
  name: string;
  requireApproval: boolean;
  freeWhen?: FreeCondition[];
}

export interface AgentGrant {
  serverId: string;
  tools: AgentToolConfig[];
  missing: boolean;
}

export interface AgentConfig {
  instructions: string;
  providerId: string;
  model: string;
  helperModel: string;
  temperature: number;
  maxSteps: number;
  maxCostCents: number;
  grants: AgentGrant[];
  providerMissing: boolean;
  tools: AgentToolConfig[];
}

export interface AgentState {
  draft: AgentConfig;
  publishedVersion: number;
  draftUpdatedAt?: Date;
  unpublishedChanges: boolean;
}

export interface Change {
  key: string;
  params?: Record<string, string | number>;
}

interface WireTool {
  name: string;
  requireApproval: boolean;
  freeWhen?: { arg: string, values: string[], absent: boolean }[];
}

interface WireConfig {
  instructions: string;
  providerId: string;
  model: string;
  helperModel: string;
  temperature: number;
  maxSteps: number;
  maxCostCents: number;
  mcpServers: { serverId: string, tools: WireTool[], missing: boolean }[];
  providerMissing: boolean;
  tools: WireTool[];
}

export interface ChangeNames {
  provider: (id: string) => string;
  server: (id: string) => string;
}

interface WireAgent {
  draft?: WireConfig;
  publishedVersion: number;
  draftUpdatedAt?: Timestamp;
  unpublishedChanges: boolean;
}

export function emptyConfig(): AgentConfig {
  return { instructions: '', providerId: '', model: '', helperModel: '', temperature: 0, maxSteps: 0, maxCostCents: 0, grants: [], providerMissing: false, tools: [] };
}

export function fromWire(c?: WireConfig): AgentConfig {
  if (!c) {
    return emptyConfig();
  }
  const tool = (t: WireTool): AgentToolConfig => ({ name: t.name, requireApproval: t.requireApproval, freeWhen: t.freeWhen?.map(f => ({ arg: f.arg, values: [...f.values], absent: f.absent })) ?? [] });
  return {
    instructions: c.instructions,
    providerId: c.providerId,
    model: c.model,
    helperModel: c.helperModel,
    temperature: c.temperature,
    maxSteps: c.maxSteps,
    maxCostCents: c.maxCostCents,
    grants: c.mcpServers.map(g => ({ serverId: g.serverId, tools: g.tools.map(tool), missing: g.missing })),
    providerMissing: c.providerMissing,
    tools: c.tools.map(tool),
  };
}

// toWire is the draft as UpdateAgentDraft takes it: never the legacy tools, the output-only flags or a grant whose server is gone.
export function toWire(c: AgentConfig) {
  return {
    instructions: c.instructions,
    providerId: c.providerId,
    model: c.model,
    helperModel: c.helperModel,
    temperature: c.temperature,
    maxSteps: c.maxSteps,
    maxCostCents: c.maxCostCents,
    mcpServers: c.grants.filter(g => !g.missing).map(g => ({ serverId: g.serverId, tools: g.tools.map(t => ({ name: t.name, requireApproval: t.requireApproval, freeWhen: t.requireApproval ? (t.freeWhen ?? []) : [] })) })),
  };
}

function grantedTools(c: AgentConfig, names: ChangeNames) {
  return new Map(c.grants.filter(g => !g.missing).flatMap(g => g.tools.map(t => [`${names.server(g.serverId)}__${t.name}`, t] as const)));
}

export function stateOf(a: WireAgent): AgentState {
  return {
    draft: fromWire(a.draft),
    publishedVersion: a.publishedVersion,
    draftUpdatedAt: a.draftUpdatedAt ? timestampDate(a.draftUpdatedAt) : undefined,
    unpublishedChanges: a.unpublishedChanges,
  };
}

// changes lists what publishing the draft changes against the live config, as locale keys under configure.change_*.
export function changes(draft: AgentConfig, live: AgentConfig | undefined, money: (cents: number) => string, names: ChangeNames): Change[] {
  if (!live) {
    return [{ key: 'configure.change_first' }];
  }
  const out: Change[] = [];
  if (draft.instructions !== live.instructions) {
    out.push({ key: 'configure.change_instructions' });
  }
  if (draft.providerId !== live.providerId || draft.model !== live.model) {
    out.push({ key: 'configure.change_provider_model', params: { provider: draft.providerId ? names.provider(draft.providerId) : '—', model: draft.model || '—' } });
  }
  if (draft.helperModel !== live.helperModel) {
    out.push({ key: 'configure.change_helper', params: { model: draft.helperModel || '—' } });
  }
  if (draft.temperature !== live.temperature) {
    out.push({ key: 'configure.change_temperature', params: { from: live.temperature, to: draft.temperature } });
  }
  if (draft.maxSteps !== live.maxSteps) {
    out.push({ key: 'configure.change_max_steps', params: { from: live.maxSteps, to: draft.maxSteps } });
  }
  if (draft.maxCostCents !== live.maxCostCents) {
    out.push({ key: 'configure.change_budget', params: { from: money(live.maxCostCents), to: money(draft.maxCostCents) } });
  }
  const before = grantedTools(live, names);
  const after = grantedTools(draft, names);
  for (const [name, tool] of after) {
    const old = before.get(name);
    if (!old) {
      out.push({ key: tool.requireApproval ? 'configure.change_tool_added_approval' : 'configure.change_tool_added', params: { tool: name } });
    } else if (old.requireApproval !== tool.requireApproval) {
      out.push({ key: tool.requireApproval ? 'configure.change_tool_needs_approval' : 'configure.change_tool_no_approval', params: { tool: name } });
    } else if (tool.requireApproval && JSON.stringify(old.freeWhen ?? []) !== JSON.stringify(tool.freeWhen ?? [])) {
      out.push({ key: 'configure.change_tool_rule', params: { tool: name } });
    }
  }
  for (const name of before.keys()) {
    if (!after.has(name)) {
      out.push({ key: 'configure.change_tool_removed', params: { tool: name } });
    }
  }
  return out;
}
