import type { ClientCache } from './cache.ts';
import type { ModelSpec } from './index.ts';
import { OpenAIModel } from '@strands-agents/sdk/models/openai';
import OpenAI from 'openai';

import { guardedFetch } from '../egress/index.ts';

export function openaiClient(cache: ClientCache<OpenAI>, project: string, spec: ModelSpec): OpenAI {
  return cache.get(`${project}:${spec.baseUrl}`, spec.apiKey, () => new OpenAI({
    baseURL: spec.baseUrl,
    apiKey: spec.apiKey,
    organization: null,
    project: null,
    maxRetries: 0,
    fetch: guardedFetch(spec.baseUrl, { header: 'authorization', value: `Bearer ${spec.apiKey}`, secrets: [`Bearer ${spec.apiKey}`, spec.apiKey] }),
  }));
}

export function openaiModel(cache: ClientCache<OpenAI>, project: string, spec: ModelSpec, temperature: number): OpenAIModel {
  return new OpenAIModel({
    api: 'chat',
    client: openaiClient(cache, project, spec),
    modelId: spec.modelId,
    temperature,
    ...(spec.maxTokens !== undefined && { maxTokens: spec.maxTokens }),
  });
}
