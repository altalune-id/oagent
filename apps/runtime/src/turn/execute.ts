import type { AgentStreamEvent, ContentBlockData, InterruptResponseContentData, JSONValue, Model, Snapshot, ToolResultBlockData } from '@strands-agents/sdk';
import type { ImageMime, TurnRequest } from '../contract/index.ts';
import type { Logger } from '../log.ts';
import type { McpTurn, Note } from '../mcp/index.ts';
import type { TurnUsage } from './limits.ts';
import {
  AfterToolCallEvent,
  Agent,
  BeforeModelCallEvent,
  ExecuteToolStage,
  Message,
  SlidingWindowConversationManager,
  TextBlock,
  ToolResultBlock,
} from '@strands-agents/sdk';

import { inputParts } from '../contract/index.ts';
import { openMcpServers } from '../mcp/index.ts';
import { historyToMessages } from './history.ts';
import { BUDGET_REFUSAL, LIMIT_REPLY, overBudget, stepLimit, UsageMeter } from './limits.ts';
import { cleanSnapshot, SDK_VERSION } from './snapshot.ts';

export type TurnStatus = 'completed' | 'awaiting_approval' | 'limit_reached' | 'cancelled' | 'failed';

export type Checkpoint = {
  toolUseId: string,
  name: string,
  input: JSONValue,
  result: { status: string, content: JSONValue[] },
};

export type ToolResultReport = Checkpoint & {
  replayed: boolean,
};

export type TurnOutcome = {
  status: TurnStatus,
  stopReason: string,
  snapshot: Snapshot | null,
  usage: TurnUsage,
  newMessages: Message[],
  interrupts: { id: string, name: string, reason: JSONValue }[],
  notes: Note[],
  error?: { code: string },
};

export interface TurnDeps {
  model: Model;
  log: Logger;
  openMcp?: (specs: TurnRequest['mcpServers'], log: Logger) => Promise<McpTurn>;
  onEvent?: (event: AgentStreamEvent) => void | Promise<void>;
  onToolResult?: (report: ToolResultReport) => void | Promise<void>;
}

const FORMAT: Record<ImageMime, 'jpeg' | 'png' | 'webp'> = { 'image/jpeg': 'jpeg', 'image/png': 'png', 'image/webp': 'webp' };

const IMAGE_REFUSAL_STATUSES = new Set([400, 413, 415, 422]);

function httpStatusOf(error: unknown): number | undefined {
  for (let e: unknown = error, depth = 0; e && typeof e === 'object' && depth < 5; e = (e as { cause?: unknown }).cause, depth++) {
    const status = (e as { status?: unknown }).status;
    if (typeof status === 'number') {
      return status;
    }
  }
  return undefined;
}

function imageRefused(request: TurnRequest, error: unknown): boolean {
  const status = httpStatusOf(error);
  const hasImage = inputParts(request).some(p => 'image' in p);
  return hasImage && status !== undefined && IMAGE_REFUSAL_STATUSES.has(status);
}

function inputOf(request: TurnRequest): ContentBlockData[] | InterruptResponseContentData[] {
  if (request.input.kind === 'message') {
    return inputParts(request).map(p => ('text' in p ? { text: p.text } : { image: { format: FORMAT[p.image.mime], source: { bytes: new Uint8Array(p.image.bytes) } } }));
  }
  if (request.input.kind === 'continue') {
    return [];
  }
  return request.input.responses.map(r => ({ interruptResponse: { interruptId: r.interruptId, response: r.response as JSONValue } }));
}

function statusOf(stopReason: string, interrupted: boolean, budgetHit: boolean): TurnStatus {
  if (interrupted) {
    return 'awaiting_approval';
  }
  if (budgetHit || stopReason.startsWith('limit')) {
    return 'limit_reached';
  }
  if (stopReason === 'cancelled') {
    return 'cancelled';
  }
  return 'completed';
}

function isBudgetRefusal(m: Message): boolean {
  return m.role === 'assistant' && m.content.length === 1 && m.content[0]!.type === 'textBlock' && m.content[0]!.text === BUDGET_REFUSAL;
}

export async function executeTurn(request: TurnRequest, signal: AbortSignal, deps: TurnDeps): Promise<TurnOutcome> {
  const openMcp = deps.openMcp ?? openMcpServers;
  const deadline = AbortSignal.timeout(request.limits.deadlineMs);
  const mcp = await openMcp(request.mcpServers, deps.log);
  const meter = new UsageMeter();
  const cancelSignal = AbortSignal.any([signal, deadline]);
  let budgetHit = false;
  const stored = new Map(request.storedResults.map(s => [s.toolUseId, s.result]));
  const replayed = new Set<string>();

  const agent = new Agent({
    model: deps.model,
    tools: mcp.tools,
    conversationManager: new SlidingWindowConversationManager({ proactiveCompression: true }),
    retryStrategy: null,
    sandbox: false,
    printer: false,
    ...(request.snapshot === null && { messages: historyToMessages(request.history) }),
  });
  if (request.snapshot !== null) {
    agent.loadSnapshot(request.snapshot as unknown as Snapshot);
  }
  agent.systemPrompt = [request.agent.instructions, ...mcp.promptLines].join('\n\n');
  const start = agent.messages.length;

  agent.addMiddleware(ExecuteToolStage.Wrap, async function* (ctx, next) {
    const result = stored.get(ctx.toolUse.toolUseId);
    if (result) {
      replayed.add(ctx.toolUse.toolUseId);
      return { result: ToolResultBlock.fromJSON({ toolResult: { toolUseId: ctx.toolUse.toolUseId, ...result } as ToolResultBlockData }) };
    }
    return yield* next(ctx);
  });
  agent.addHook(BeforeModelCallEvent, (event) => {
    if (overBudget(meter.usage, request.limits.budget)) {
      budgetHit = true;
      event.cancel = BUDGET_REFUSAL;
      return;
    }
    meter.startCall(JSON.stringify(agent.messages).length + String(agent.systemPrompt ?? '').length);
  });
  agent.addHook(AfterToolCallEvent, async (event) => {
    if (!deps.onToolResult) {
      return;
    }
    const { toolUseId: _id, ...result } = event.result.toJSON().toolResult;
    await deps.onToolResult({
      toolUseId: event.toolUse.toolUseId,
      name: event.toolUse.name,
      input: event.toolUse.input,
      result: result as unknown as Checkpoint['result'],
      replayed: replayed.has(event.toolUse.toolUseId),
    });
  });

  const outcome = (status: TurnStatus, stopReason: string, extra: Partial<TurnOutcome> = {}): TurnOutcome => ({
    status,
    stopReason,
    snapshot: null,
    usage: meter.usage,
    newMessages: agent.messages.slice(start).filter(m => !(budgetHit && isBudgetRefusal(m))),
    interrupts: [],
    notes: mcp.notes,
    ...extra,
  });

  try {
    const stream = agent.stream(inputOf(request), { limits: { turns: stepLimit(request.agent.maxSteps) }, cancelSignal });
    let next = await stream.next();
    while (!next.done) {
      const event = next.value;
      if (event.type === 'modelStreamUpdateEvent') {
        const inner = event.event;
        if (inner.type === 'modelMetadataEvent' && inner.usage) {
          meter.addMetadata(inner.usage);
        }
        if (inner.type === 'modelContentBlockDeltaEvent' && inner.delta.type === 'textDelta') {
          meter.addOutput(inner.delta.text);
        }
      }
      await deps.onEvent?.(event);
      next = await stream.next();
    }
    const result = next.value;
    meter.closeCutCall();
    const interrupts = (result.interrupts ?? []).map(i => ({ id: i.id, name: i.name, reason: (i.reason ?? null) as JSONValue }));
    const status = statusOf(result.stopReason, interrupts.length > 0, budgetHit);
    const snapshot = cleanSnapshot(agent.takeSnapshot({ preset: 'session', appData: { thread: request.ids.thread, agentVersion: request.ids.agentVersion, sdk: SDK_VERSION } }), budgetHit ? BUDGET_REFUSAL : undefined);
    const done = outcome(status, budgetHit ? 'budget' : result.stopReason, { snapshot, interrupts });
    if (status === 'cancelled') {
      done.newMessages = done.newMessages.filter(m => m !== result.lastMessage);
    }
    if (budgetHit) {
      done.newMessages.push(new Message({ role: 'assistant', content: [new TextBlock(LIMIT_REPLY)] }));
    }
    if (status === 'cancelled' && deadline.aborted && !signal.aborted) {
      done.error = { code: 'deadline' };
    }
    return done;
  } catch (error) {
    meter.closeFailedCall();
    deps.log.error({ run: request.ids.run, error }, 'turn failed');
    return outcome('failed', 'error', { error: { code: imageRefused(request, error) ? 'image_unsupported' : 'model_error' } });
  } finally {
    await mcp.close();
  }
}
