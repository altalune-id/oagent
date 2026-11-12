import type { Message } from '@strands-agents/sdk';
import type { ApprovalSpec } from '../contract/index.ts';
import { InterventionHandler } from '@strands-agents/sdk';

import { argsHash } from '../contract/index.ts';
import { evaluate, orderedKeys } from './rule.ts';

export const APPROVAL_HANDLER = 'oagent:approval';
export const MAX_GATED_PER_STEP = 8;
export const TOO_MANY = 'too many actions at once; ask again';
const MISMATCH = 'this approval does not match the call';
const INVALID = 'this approval is not valid';
export const NO_SERVER = 'this tool is not granted to the agent';

type BeforeToolCall = Parameters<InterventionHandler['beforeToolCall']>[0];

export type ToolGate = {
  server: string,
  tool: string,
  approval: ApprovalSpec,
};

type Answer = {
  approved: boolean,
  text?: string,
  argsHash: string,
};

function isAnswer(value: unknown): value is Answer {
  const a = value as Partial<Answer> | null;
  return a !== null && typeof a === 'object' && typeof a.approved === 'boolean' && typeof a.argsHash === 'string'
    && (a.text === undefined || typeof a.text === 'string');
}

export class ApprovalCheck extends InterventionHandler {
  readonly name = APPROVAL_HANDLER;
  override readonly onError = 'deny' as const;
  private readonly specs: Map<string, ToolGate>;
  private readonly gated = new WeakMap<object, Set<string>>();

  constructor(specs: Map<string, ToolGate>) {
    super();
    this.specs = specs;
  }

  override beforeToolCall(event: BeforeToolCall) {
    if (event.tool === undefined) {
      return { type: 'proceed' as const };
    }
    const gate = this.specs.get(event.toolUse.name);
    if (gate === undefined) {
      return { type: 'deny' as const, reason: NO_SERVER };
    }
    const schema = event.tool?.toolSpec.inputSchema;
    const input = (event.toolUse.input ?? {}) as Record<string, unknown>;
    if (evaluate(gate.approval, schema, input) === 'free') {
      return { type: 'proceed' as const };
    }
    if (!this.admit(event)) {
      return { type: 'deny' as const, reason: TOO_MANY };
    }
    const hash = argsHash(input);
    const reason = { toolUseId: event.toolUse.toolUseId, server: gate.server, tool: gate.tool, input, argsHash: hash, keys: orderedKeys(input, schema) };
    const answer: unknown = event.interrupt({ name: this.name, reason: reason as never });
    if (!isAnswer(answer)) {
      return { type: 'deny' as const, reason: INVALID };
    }
    if (answer.argsHash !== hash) {
      return { type: 'deny' as const, reason: MISMATCH };
    }
    if (answer.approved) {
      return { type: 'proceed' as const };
    }
    return { type: 'deny' as const, reason: answer.text || 'rejected' };
  }

  private admit(event: BeforeToolCall): boolean {
    const step: object = (event.agent.messages.at(-1) as Message | undefined) ?? event.agent;
    const seen = this.gated.get(step) ?? new Set<string>();
    this.gated.set(step, seen);
    if (seen.has(event.toolUse.toolUseId)) {
      return true;
    }
    if (seen.size >= MAX_GATED_PER_STEP) {
      return false;
    }
    seen.add(event.toolUse.toolUseId);
    return true;
  }
}
