import Anthropic from '@anthropic-ai/sdk';
import { AnthropicModel } from '@strands-agents/sdk/models/anthropic';
import { OpenAIModel } from '@strands-agents/sdk/models/openai';
import OpenAI from 'openai';
import { describe, expect, it } from 'vitest';

import { createModels } from './index.ts';

const spec = { provider: 'openai_compatible' as const, baseUrl: 'https://llm.example.com/v1', apiKey: 'sk-test', modelId: 'gpt-4o' };

describe('createModels', () => {
  it('builds an OpenAI-compatible chat model for the spec', () => {
    const model = createModels().model('p1', spec, 0.3);
    expect(model).toBeInstanceOf(OpenAIModel);
    expect(model.getConfig()).toMatchObject({ modelId: 'gpt-4o', temperature: 0.3 });
  });

  it('builds an Anthropic model, without the temperature, for an Anthropic spec', () => {
    const model = createModels().model('p1', { ...spec, provider: 'anthropic', baseUrl: 'https://api.anthropic.com', modelId: 'claude-sonnet-5-5' }, 0.3);
    expect(model).toBeInstanceOf(AnthropicModel);
    expect(model.getConfig()).toMatchObject({ modelId: 'claude-sonnet-5-5' });
    expect(model.getConfig()).not.toHaveProperty('temperature');
  });

  it('keeps one client cache per provider', () => {
    const anthropicSpec = { ...spec, provider: 'anthropic' as const };
    const clientOf = (model: unknown): unknown => (model as { _client: unknown })._client;
    const models = createModels();
    expect(clientOf(models.model('p1', anthropicSpec, 0))).toBeInstanceOf(Anthropic);
    expect(clientOf(models.model('p1', spec, 0))).toBeInstanceOf(OpenAI);
    expect(clientOf(models.model('p1', anthropicSpec, 0))).toBeInstanceOf(Anthropic);
  });

  it('throws on an unknown provider instead of building a broken model', () => {
    const broken = { ...spec, provider: 'bedrock' } as unknown as typeof spec;
    expect(() => createModels().model('p1', broken, 0)).toThrow('models: unknown provider bedrock');
  });
});
