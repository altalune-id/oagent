import type { Timestamp } from '@bufbuild/protobuf/wkt';
import { timestampDate } from '@bufbuild/protobuf/wkt';

export interface AgentToolConfig {
  name: string;
  requireApproval: boolean;
}

export interface AgentConfig {
  instructions: string;
  model: string;
  temperature: number;
  maxSteps: number;
  maxCostCents: number;
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

interface WireConfig {
  instructions: string;
  model: string;
  temperature: number;
  maxSteps: number;
  maxCostCents: number;
  tools: { name: string, requireApproval: boolean }[];
}

interface WireAgent {
  draft?: WireConfig;
  publishedVersion: number;
  draftUpdatedAt?: Timestamp;
  unpublishedChanges: boolean;
}

export function emptyConfig(): AgentConfig {
  return { instructions: '', model: '', temperature: 0, maxSteps: 0, maxCostCents: 0, tools: [] };
}

export function fromWire(c?: WireConfig): AgentConfig {
  if (!c) {
    return emptyConfig();
  }
  return {
    instructions: c.instructions,
    model: c.model,
    temperature: c.temperature,
    maxSteps: c.maxSteps,
    maxCostCents: c.maxCostCents,
    tools: c.tools.map(t => ({ name: t.name, requireApproval: t.requireApproval })),
  };
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
export function changes(draft: AgentConfig, live: AgentConfig | undefined, money: (cents: number) => string): Change[] {
  if (!live) {
    return [{ key: 'configure.change_first' }];
  }
  const out: Change[] = [];
  if (draft.instructions !== live.instructions) {
    out.push({ key: 'configure.change_instructions' });
  }
  if (draft.model !== live.model) {
    out.push({ key: 'configure.change_model', params: { from: live.model || '—', to: draft.model || '—' } });
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
  const before = new Map(live.tools.map(t => [t.name, t]));
  const after = new Map(draft.tools.map(t => [t.name, t]));
  for (const [name, tool] of after) {
    const old = before.get(name);
    if (!old) {
      out.push({ key: tool.requireApproval ? 'configure.change_tool_added_approval' : 'configure.change_tool_added', params: { tool: name } });
    } else if (old.requireApproval !== tool.requireApproval) {
      out.push({ key: tool.requireApproval ? 'configure.change_tool_needs_approval' : 'configure.change_tool_no_approval', params: { tool: name } });
    }
  }
  for (const name of before.keys()) {
    if (!after.has(name)) {
      out.push({ key: 'configure.change_tool_removed', params: { tool: name } });
    }
  }
  return out;
}
