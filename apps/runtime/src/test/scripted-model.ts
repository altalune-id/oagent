import type { BaseModelConfig, Message, ModelStreamEvent, StreamOptions } from '@strands-agents/sdk';
import { Model } from '@strands-agents/sdk';

export type Step = {
  text?: string[],
  toolUse?: { name: string, id: string, input: Record<string, unknown> },
  toolUses?: { name: string, id: string, input: Record<string, unknown> }[],
  usage?: { inputTokens: number, outputTokens: number, cacheReadInputTokens?: number, cacheWriteInputTokens?: number },
  hang?: boolean,
  error?: Error,
  failAfterText?: Error,
};

export type Call = {
  messages: Message[],
  systemPrompt: unknown,
};

function abortError(): Error {
  const e = new Error('aborted');
  e.name = 'AbortError';
  return e;
}

export class ScriptedModel extends Model<BaseModelConfig> {
  readonly calls: Call[] = [];
  private config: BaseModelConfig = { modelId: 'scripted' };
  private readonly steps: Step[];

  constructor(steps: Step[]) {
    super();
    this.steps = steps;
  }

  updateConfig(config: BaseModelConfig): void {
    this.config = { ...this.config, ...config };
  }

  getConfig(): BaseModelConfig {
    return this.config;
  }

  async* stream(messages: Message[], options?: StreamOptions): AsyncIterable<ModelStreamEvent> {
    this.calls.push({ messages: [...messages], systemPrompt: options?.systemPrompt });
    const step = this.steps.shift();
    if (!step) {
      throw new Error('scripted model: no step left');
    }
    if (step.error) {
      throw step.error;
    }
    yield { type: 'modelMessageStartEvent', role: 'assistant' };
    if (step.text) {
      yield { type: 'modelContentBlockStartEvent' };
      for (const chunk of step.text) {
        yield { type: 'modelContentBlockDeltaEvent', delta: { type: 'textDelta', text: chunk } };
      }
    }
    if (step.hang) {
      await new Promise<void>((_resolve, reject) => {
        const signal = options?.cancelSignal;
        if (signal?.aborted) {
          reject(abortError());
          return;
        }
        signal?.addEventListener('abort', () => reject(abortError()), { once: true });
      });
    }
    if (step.failAfterText) {
      throw step.failAfterText;
    }
    if (step.text) {
      yield { type: 'modelContentBlockStopEvent' };
    }
    for (const use of [...(step.toolUse ? [step.toolUse] : []), ...(step.toolUses ?? [])]) {
      yield { type: 'modelContentBlockStartEvent', start: { type: 'toolUseStart', name: use.name, toolUseId: use.id } };
      yield { type: 'modelContentBlockDeltaEvent', delta: { type: 'toolUseInputDelta', input: JSON.stringify(use.input) } };
      yield { type: 'modelContentBlockStopEvent' };
    }
    yield { type: 'modelMessageStopEvent', stopReason: step.toolUse || step.toolUses?.length ? 'toolUse' : 'endTurn' };
    if (step.usage) {
      yield { type: 'modelMetadataEvent', usage: { totalTokens: step.usage.inputTokens + step.usage.outputTokens, ...step.usage } };
    }
  }
}
