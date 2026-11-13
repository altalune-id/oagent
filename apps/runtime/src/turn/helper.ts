import type { BaseModelConfig, CountTokensOptions, Message, ModelInfo, ModelStreamEvent, StreamOptions } from '@strands-agents/sdk';
import type { TurnRequest } from '../contract/index.ts';
import type { Models } from '../models/index.ts';
import type { Budget, TurnUsage } from './limits.ts';
import { Model } from '@strands-agents/sdk';

import { addHelperUsage, overBudget, UsageMeter } from './limits.ts';

export class HelperBudgetError extends Error {
  override name = 'HelperBudgetError';
}

class MeteredModel extends Model<BaseModelConfig> {
  readonly #inner: Model;
  readonly #usage: TurnUsage;
  readonly #budget: Budget | null;

  constructor(inner: Model, usage: TurnUsage, budget: Budget | null) {
    super();
    this.#inner = inner;
    this.#usage = usage;
    this.#budget = budget;
  }

  updateConfig(config: BaseModelConfig): void {
    this.#inner.updateConfig(config);
  }

  getConfig(): BaseModelConfig {
    return this.#inner.getConfig();
  }

  override get modelInfo(): ModelInfo {
    return this.#inner.modelInfo;
  }

  override countTokens(messages: Message[], options?: CountTokensOptions): Promise<number> {
    return this.#inner.countTokens(messages, options);
  }

  async* stream(messages: Message[], options?: StreamOptions): AsyncIterable<ModelStreamEvent> {
    if (overBudget(this.#usage, this.#budget)) {
      throw new HelperBudgetError('helper budget exhausted');
    }
    const call = new UsageMeter();
    call.startCall(JSON.stringify(messages).length + String(options?.systemPrompt ?? '').length);
    try {
      for await (const event of this.#inner.stream(messages, options)) {
        if (event.type === 'modelMetadataEvent' && event.usage) {
          call.addMetadata(event.usage);
        }
        if (event.type === 'modelContentBlockDeltaEvent' && event.delta.type === 'textDelta') {
          call.addOutput(event.delta.text);
        }
        yield event;
      }
    } catch (error) {
      call.closeFailedCall();
      throw error;
    } finally {
      call.closeCutCall();
      addHelperUsage(this.#usage, call.usage);
    }
  }
}

export function helperModel(request: TurnRequest, models: Models, usage: TurnUsage, maxTokens?: number): Model {
  const base = request.agent.helperModel ?? request.agent.model;
  const spec = maxTokens === undefined ? base : { ...base, maxTokens };
  return new MeteredModel(models.model(request.ids.project, spec, request.agent.temperature), usage, request.limits.budget);
}
