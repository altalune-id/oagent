import type { AgentStreamEvent, Message } from '@strands-agents/sdk';
import type { Emit, TurnRequest } from '../contract/index.ts';
import type { ToolResultReport } from './execute.ts';

import { IMAGE_NOTE } from './snapshot.ts';

type Json = Record<string, unknown>;

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

function withoutImageBytes<T>(content: T): T {
  if (!Array.isArray(content)) {
    return content;
  }
  return content.map(p => ((p as { type?: string }).type === 'text' ? p : { type: 'text', text: IMAGE_NOTE })) as T;
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
      out.push({ id: request.input.message.id, role: 'user', content: withoutImageBytes(request.input.message.content) });
      continue;
    }
    out.push({ id: `msg_user_${users}`, role: 'user', content: m.content.filter(b => b.type === 'textBlock').map(b => b.text).join('') });
  }
  return out;
}

export class StreamMapper {
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
