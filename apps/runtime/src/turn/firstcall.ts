import type { Agent } from '@strands-agents/sdk';
import { InvokeModelStage, Message, MessageAddedEvent, TextBlock } from '@strands-agents/sdk';

export function injectOnFirstCall(agent: Agent, block: string, onInjected: (chars: number) => void): void {
  if (block === '') {
    return;
  }
  const text = `\n\n${block}`;
  let delivered = false;
  let injectedThisCall = false;
  agent.addMiddleware(InvokeModelStage.Input, (ctx) => {
    const last = ctx.messages.at(-1);
    injectedThisCall = !delivered && last?.role === 'user';
    if (!injectedThisCall || last === undefined) {
      return ctx;
    }
    const messages = [...ctx.messages];
    messages[messages.length - 1] = new Message({
      role: 'user',
      content: [...last.content, new TextBlock(text)],
      trackingId: last.trackingId,
      ...(last.metadata !== undefined && { metadata: last.metadata }),
    });
    onInjected(text.length);
    return { ...ctx, messages, dynamicTrailingBlocks: (ctx.dynamicTrailingBlocks ?? 0) + 1 };
  });
  agent.addHook(MessageAddedEvent, (event) => {
    if (injectedThisCall && event.message.role === 'assistant') {
      delivered = true;
    }
  });
}
