import type { ContentBlockData, JSONValue, MessageData } from '@strands-agents/sdk';
import type { TurnRequest } from '../contract/index.ts';

type AgUiMessage = TurnRequest['history'][number];

function textOf(content: unknown): string {
  if (typeof content === 'string') {
    return content;
  }
  if (Array.isArray(content)) {
    return content.map(p => (p && typeof p === 'object' && (p as { type?: string }).type === 'text' ? String((p as { text?: unknown }).text ?? '') : '')).join('');
  }
  return '';
}

export function historyToMessages(history: AgUiMessage[]): MessageData[] {
  const out: MessageData[] = [];
  for (const m of history) {
    if (m.role === 'user') {
      out.push({ role: 'user', content: [{ text: textOf(m.content) }] });
      continue;
    }
    if (m.role === 'assistant') {
      const content: ContentBlockData[] = [];
      const text = textOf(m.content);
      if (text) {
        content.push({ text });
      }
      for (const call of m.toolCalls ?? []) {
        content.push({ toolUse: { name: call.function.name, toolUseId: call.id, input: JSON.parse(call.function.arguments || '{}') as JSONValue } });
      }
      out.push({ role: 'assistant', content });
      continue;
    }
    if (m.role === 'tool') {
      const block: ContentBlockData = { toolResult: { toolUseId: m.toolCallId, status: m.error ? 'error' : 'success', content: [{ text: textOf(m.content) }] } };
      const last = out.at(-1);
      if (last?.role === 'user' && last.content.every(c => 'toolResult' in c)) {
        last.content.push(block);
        continue;
      }
      out.push({ role: 'user', content: [block] });
    }
  }
  return out;
}
