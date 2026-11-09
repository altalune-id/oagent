import type Anthropic from '@anthropic-ai/sdk';
import type { Model } from '@strands-agents/sdk';
import type OpenAI from 'openai';
import type { TurnRequest } from '../contract/index.ts';
import type { CacheOptions } from './cache.ts';

import { ModelProvider } from '../contract/index.ts';
import { anthropicModel } from './anthropic.ts';
import { ClientCache } from './cache.ts';
import { openaiModel } from './openai.ts';

export type ModelSpec = TurnRequest['agent']['model'];

export interface Models {
  model: (project: string, spec: ModelSpec, temperature: number) => Model;
}

export function createModels(options: CacheOptions = {}): Models {
  const openai = new ClientCache<OpenAI>(options);
  const anthropic = new ClientCache<Anthropic>(options);
  return {
    model: (project, spec, temperature) => {
      switch (spec.provider) {
        case ModelProvider.OPENAI_COMPATIBLE:
          return openaiModel(openai, project, spec, temperature);
        case ModelProvider.ANTHROPIC:
          return anthropicModel(anthropic, project, spec);
        default:
          return assertNever(spec.provider, 'models: unknown provider');
      }
    },
  };
}

function assertNever(value: never, message: string): never {
  throw new Error(`${message} ${String(value)}`);
}
