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

export function anthropicModel(cache: ClientCache<Anthropic>, project: string, spec: ModelSpec): AnthropicModel {
  return new AnthropicModel({
    client: anthropicClient(cache, project, spec),
    modelId: spec.modelId,
    cacheConfig: { strategy: 'anthropic' },
    params: { thinking: { type: 'adaptive' } },
    ...(spec.maxTokens !== undefined && { maxTokens: spec.maxTokens }),
  });
}
