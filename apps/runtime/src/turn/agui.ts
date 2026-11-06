import type { AgentStreamEvent, Message, Model } from '@strands-agents/sdk';
import type { TurnRequest } from '../http/contract.ts';
import type { Emit, RunTurn } from '../http/server.ts';
import type { Logger } from '../log.ts';
import type { ModelClients } from '../models/clients.ts';
import type { ToolResultReport, TurnDeps, TurnOutcome } from './agent.ts';

import { checkSnapshot, executeTurn } from './agent.ts';
import { emptyUsage, LIMIT_REPLY } from './limits.ts';

export const HEARTBEAT_MS = 15_000;

const ERROR_TEXT: Record<string, string> = {
  deadline: 'turn deadline reached',
  model_error: 'model call failed',
  runtime_error: 'runtime error',
};

type Json = Record<string, unknown>;

export interface RunTurnDeps {
  models: ModelClients;
  log: Logger;
  modelFor?: (request: TurnRequest) => Model;
  openMcp?: TurnDeps['openMcp'];
}

function contentText(content: unknown[]): string {
  return content.map((block) => {
    const b = block as { text?: unknown, json?: unknown };
    if (typeof b.text === 'string') {
      return b.text;
    }
    if (b.json !== undefined) {
      return JSON.stringify(b.json);
    }
    return '';
  }).join('\n');
}

export function toAgUiMessages(request: TurnRequest, messages: Message[]): Json[] {
  const out: Json[] = [];
  let assistants = 0;
  let users = 0;
  for (const m of messages) {
    if (m.role === 'assistant') {
      assistants++;
      const text = m.content.filter(b => b.type === 'textBlock').map(b => b.text).join('');
      const calls = m.content.filter(b => b.type === 'toolUseBlock').map(b => ({
        id: b.toolUseId,
        type: 'function',
        function: { name: b.name, arguments: JSON.stringify(b.input) },
      }));
      out.push({ id: `msg_asst_${assistants}`, role: 'assistant', ...(text && { content: text }), ...(calls.length > 0 && { toolCalls: calls }) });
      continue;
    }
    const results = m.content.filter(b => b.type === 'toolResultBlock');
    if (results.length > 0) {
      for (const r of results) {
        const data = r.toJSON().toolResult;
        out.push({ id: `msg_tool_${data.toolUseId}`, role: 'tool', toolCallId: data.toolUseId, content: contentText(data.content) });
      }
      continue;
    }
    users++;
    if (users === 1 && request.input.kind === 'message') {
      out.push({ id: request.input.message.id, role: 'user', content: request.input.message.content });
      continue;
    }
    out.push({ id: `msg_user_${users}`, role: 'user', content: m.content.filter(b => b.type === 'textBlock').map(b => b.text).join('') });
  }
  return out;
}

class StreamMapper {
  private assistants = 0;
  private current = '';
  private textOpen = false;
  private streamedSinceTools = false;
  private readonly emit: Emit;

  constructor(emit: Emit) {
    this.emit = emit;
  }

  private nextAssistant(): string {
    this.assistants++;
    this.current = `msg_asst_${this.assistants}`;
    return this.current;
  }

  private async closeText(): Promise<void> {
    if (this.textOpen) {
      this.textOpen = false;
      await this.emit({ type: 'TEXT_MESSAGE_END', messageId: this.current });
    }
  }

  async onEvent(event: AgentStreamEvent): Promise<void> {
    if (event.type === 'modelStreamUpdateEvent') {
      const inner = event.event;
      if (inner.type === 'modelMessageStartEvent') {
        this.nextAssistant();
        this.streamedSinceTools = true;
        return;
      }
      if (inner.type === 'modelContentBlockDeltaEvent' && inner.delta.type === 'textDelta') {
        if (!this.textOpen) {
          this.textOpen = true;
          await this.emit({ type: 'TEXT_MESSAGE_START', messageId: this.current, role: 'assistant' });
        }
        await this.emit({ type: 'TEXT_MESSAGE_CONTENT', messageId: this.current, delta: inner.delta.text });
        return;
      }
      if (inner.type === 'modelContentBlockStopEvent' || inner.type === 'modelMessageStopEvent') {
        await this.closeText();
      }
      return;
    }
    if (event.type === 'beforeToolsEvent') {
      if (!this.streamedSinceTools) {
        this.nextAssistant();
      }
      this.streamedSinceTools = false;
      return;
    }
    if (event.type === 'beforeToolCallEvent') {
      const { toolUseId, name, input } = event.toolUse;
      await this.emit({ type: 'TOOL_CALL_START', toolCallId: toolUseId, toolCallName: name, parentMessageId: this.current });
      await this.emit({ type: 'TOOL_CALL_ARGS', toolCallId: toolUseId, delta: JSON.stringify(input) });
      await this.emit({ type: 'TOOL_CALL_END', toolCallId: toolUseId });
    }
  }

  async onToolResult(report: ToolResultReport): Promise<void> {
    await this.emit({ type: 'TOOL_CALL_RESULT', messageId: `msg_tool_${report.toolUseId}`, toolCallId: report.toolUseId, content: contentText(report.result.content), role: 'tool' });
    if (!report.replayed) {
      await this.emit({ type: 'OAGENT_CHECKPOINT', toolUseId: report.toolUseId, name: report.name, input: report.input, result: report.result });
    }
  }

  async reply(text: string): Promise<void> {
    const messageId = this.nextAssistant();
    await this.emit({ type: 'TEXT_MESSAGE_START', messageId, role: 'assistant' });
    await this.emit({ type: 'TEXT_MESSAGE_CONTENT', messageId, delta: text });
    await this.emit({ type: 'TEXT_MESSAGE_END', messageId });
  }
}

function runtimeFailure(): TurnOutcome {
  return {
    status: 'failed',
    stopReason: 'error',
    snapshot: null,
    usage: emptyUsage(),
    newMessages: [],
    interrupts: [],
    notes: [],
    error: { code: 'runtime_error' },
  };
}

export function createRunTurn(deps: RunTurnDeps): RunTurn {
  return async (request, emit, signal) => {
    checkSnapshot(request);
    const model = deps.modelFor?.(request) ?? deps.models.model(request.ids.project, request.agent.model, request.agent.temperature);
    const mapper = new StreamMapper(emit);
    const ids = { threadId: request.ids.thread, runId: request.ids.run };
    await emit({ type: 'RUN_STARTED', ...ids });
    const beat = setInterval(() => {
      void emit({ type: 'OAGENT_HEARTBEAT' });
    }, HEARTBEAT_MS);
    let outcome: TurnOutcome;
    try {
      outcome = await executeTurn(request, signal, {
        model,
        log: deps.log,
        ...(deps.openMcp && { openMcp: deps.openMcp }),
        onEvent: e => mapper.onEvent(e),
        onToolResult: r => mapper.onToolResult(r),
      });
    } catch (error) {
      deps.log.error('turn crashed', { run: request.ids.run, error });
      outcome = runtimeFailure();
    } finally {
      clearInterval(beat);
    }
    if (outcome.stopReason === 'budget') {
      await mapper.reply(LIMIT_REPLY);
    }
    if (outcome.error) {
      await emit({ type: 'RUN_ERROR', message: ERROR_TEXT[outcome.error.code] ?? 'turn failed', code: outcome.error.code });
    } else {
      await emit({ type: 'RUN_FINISHED', ...ids });
    }
    await emit({
      type: 'OAGENT_TERMINAL',
      status: outcome.status,
      stopReason: outcome.stopReason,
      snapshot: outcome.snapshot,
      usage: outcome.usage,
      messagesAppended: toAgUiMessages(request, outcome.newMessages),
      interrupts: outcome.interrupts,
      notes: outcome.notes,
      ...(outcome.error && { error: outcome.error }),
    });
  };
}
