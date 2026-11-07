import type { Model } from '@strands-agents/sdk';
import type OpenAI from 'openai';
import type { CacheOptions } from './cache.ts';

import { ClientCache } from './cache.ts';
import { openaiModel } from './openai.ts';

export type ModelSpec = {
  baseUrl: string,
  apiKey: string,
  modelId: string,
  maxTokens?: number,
};

export interface Models {
  model: (project: string, spec: ModelSpec, temperature: number) => Model;
}

export function createModels(options: CacheOptions = {}): Models {
  const cache = new ClientCache<OpenAI>(options);
  return {
    model: (project, spec, temperature) => openaiModel(cache, project, spec, temperature),
  };
}
