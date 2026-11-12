import type { Message, ModelStreamEvent, StreamOptions } from '@strands-agents/sdk';
import type { AnthropicModelOptions } from '@strands-agents/sdk/models/anthropic';
import type { ClientCache } from './cache.ts';
import type { ModelSpec } from './index.ts';
import Anthropic from '@anthropic-ai/sdk';
import { AnthropicModel } from '@strands-agents/sdk/models/anthropic';

import { guardedFetch } from '../egress/index.ts';

export function anthropicClient(cache: ClientCache<Anthropic>, project: string, spec: ModelSpec): Anthropic {
  return cache.get(`${project}:${spec.baseUrl}`, spec.apiKey, () => new Anthropic({
    baseURL: spec.baseUrl,
    apiKey: spec.apiKey,
    authToken: null,
    maxRetries: 0,
    logLevel: 'off',
    fetch: guardedFetch(spec.baseUrl, { header: 'x-api-key', value: spec.apiKey, secrets: [spec.apiKey] }),
  }));
}

// NOTE: deny-list, not allow-list: an unread id keeps thinking so Strands' drop_block retry still covers models that think upstream.
const PRE_ADAPTIVE = /claude-(?:[0-3](?=$|[-.@:])|instant|[a-z]+-[0-3](?=$|[-.@:])|[a-z]+-4(?:-[0-5])?(?=$|[@:.]|-\d{8}|-v\d))/;

export function supportsAdaptiveThinking(modelId: string): boolean {
  return !PRE_ADAPTIVE.test(modelId);
}

type CallSignal = { current?: AbortSignal };

// NOTE: Strands' AnthropicModel does not pass its stream's cancelSignal to the SDK; this wrapper does, so a closed turn aborts the model call.
class CancellableAnthropicModel extends AnthropicModel {
  readonly #call: CallSignal;

  constructor(options: AnthropicModelOptions, call: CallSignal) {
    super(options);
    this.#call = call;
  }

  override async* stream(messages: Message[], options?: StreamOptions): AsyncGenerator<ModelStreamEvent> {
    this.#call.current = options?.cancelSignal;
    try {
      yield* super.stream(messages, options);
    } finally {
      this.#call.current = undefined;
    }
  }
}

function withCallSignal(client: Anthropic, call: CallSignal): Anthropic {
  const messages = new Proxy(client.messages, {
    get(target, prop, receiver) {
      const value: unknown = Reflect.get(target, prop, receiver);
      if (typeof value !== 'function' || !['create', 'stream', 'countTokens'].includes(String(prop))) {
        return value;
      }
      return (params: unknown, options?: Record<string, unknown>) =>
        (value as (p: unknown, o?: Record<string, unknown>) => unknown).call(target, params, call.current ? { ...options, signal: call.current } : options);
    },
  });
  return new Proxy(client, {
    get: (target, prop, receiver) => (prop === 'messages' ? messages : Reflect.get(target, prop, receiver)),
  });
}

export function anthropicModel(cache: ClientCache<Anthropic>, project: string, spec: ModelSpec): AnthropicModel {
  const call: CallSignal = {};
  return new CancellableAnthropicModel({
    client: withCallSignal(anthropicClient(cache, project, spec), call),
    modelId: spec.modelId,
    cacheConfig: { strategy: 'anthropic' },
    ...(supportsAdaptiveThinking(spec.modelId) && { params: { thinking: { type: 'adaptive' } } }),
    ...(spec.maxTokens !== undefined && { maxTokens: spec.maxTokens }),
  }, call);
}
