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

// shortcut: reads the version from claude-<family>-<major>[-<minor>] ids only; other ids get no thinking, add a rule when one needs it.
const VERSIONED_ID = /^claude-[a-z]+-(\d+)(?:-(\d{1,2}))?(?:-|$)/;

export function supportsAdaptiveThinking(modelId: string): boolean {
  const match = VERSIONED_ID.exec(modelId);
  if (!match) {
    return false;
  }
  const major = Number(match[1]);
  return major > 4 || (major === 4 && Number(match[2] ?? 0) >= 6);
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
