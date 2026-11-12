import type { Snapshot } from '@strands-agents/sdk';
import type { TurnRequest } from '../contract/index.ts';
import type { McpTurn } from '../mcp/index.ts';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { Agent, FunctionTool, InterventionHandler } from '@strands-agents/sdk';

import { validateRequest } from '../contract/index.ts';
import { captureLogger } from './logger.ts';
import { ScriptedModel } from './scripted-model.ts';

export const quiet = captureLogger();

export function oldGoRequest(name: string): TurnRequest {
  const path = fileURLToPath(new URL(`./fixtures/${name}.request.json`, import.meta.url));
  const result = validateRequest(JSON.parse(readFileSync(path, 'utf8')));
  if (!result.ok) {
    throw new Error(`old Go request ${name} refused: ${result.fields.join(',')}`);
  }
  return result.request;
}

export function goldenRequest(name: string, over: (r: TurnRequest) => void = () => {}): TurnRequest {
  const path = fileURLToPath(new URL(`../../../../internal/kernel/runtime/testdata/turn/${name}.request.json`, import.meta.url));
  const result = validateRequest(JSON.parse(readFileSync(path, 'utf8')));
  if (!result.ok) {
    throw new Error(`golden ${name} invalid: ${result.fields.join(',')}`);
  }
  over(result.request);
  return result.request;
}

export function recordTool(calls: unknown[]): FunctionTool {
  return new FunctionTool({
    name: 'yasaku__record_expense',
    description: 'Record an expense.',
    inputSchema: { type: 'object', properties: { amount: { type: 'number' } } },
    callback: (input) => {
      calls.push(input);
      return { id: 'exp_1' };
    },
  });
}

export function fakeMcp(tools: FunctionTool[], extra: Partial<McpTurn> = {}): () => Promise<McpTurn> {
  return async () => ({ servers: [], tools: tools as never, notes: [], promptLines: [], close: async () => {}, ...extra });
}

export class AskFirst extends InterventionHandler {
  readonly name: string;

  constructor(name = 'test:approval') {
    super();
    this.name = name;
  }

  override beforeToolCall(event: Parameters<InterventionHandler['beforeToolCall']>[0]) {
    event.interrupt({ name: this.name, reason: { tool: event.toolUse.name } });
    return { type: 'proceed' as const };
  }
}

export async function pendingSnapshot(name: string, appData: Record<string, unknown>, input: Record<string, unknown> = {}): Promise<Snapshot> {
  const agent = new Agent({
    model: new ScriptedModel([{ toolUse: { name: 'yasaku__record_expense', id: 'call_1', input } }]),
    tools: [recordTool([])],
    interventions: [new AskFirst(name)],
    sandbox: false,
    printer: false,
  });
  await agent.invoke('record lunch 25000 from cash');
  return agent.takeSnapshot({ preset: 'session', appData: appData as never });
}
