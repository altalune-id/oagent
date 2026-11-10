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

export function anthropicModel(cache: ClientCache<Anthropic>, project: string, spec: ModelSpec): AnthropicModel {
  return new AnthropicModel({
    client: anthropicClient(cache, project, spec),
    modelId: spec.modelId,
    cacheConfig: { strategy: 'anthropic' },
    ...(supportsAdaptiveThinking(spec.modelId) && { params: { thinking: { type: 'adaptive' } } }),
    ...(spec.maxTokens !== undefined && { maxTokens: spec.maxTokens }),
  });
}
