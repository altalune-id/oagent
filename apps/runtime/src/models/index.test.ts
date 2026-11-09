import { AnthropicModel } from '@strands-agents/sdk/models/anthropic';
import { OpenAIModel } from '@strands-agents/sdk/models/openai';
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
    const models = createModels();
    const a = models.model('p1', { ...spec, provider: 'anthropic', baseUrl: 'https://llm.example.com/v1' }, 0);
    const o = models.model('p1', spec, 0);
    expect(a).toBeInstanceOf(AnthropicModel);
    expect(o).toBeInstanceOf(OpenAIModel);
  });

  it('throws on an unknown provider instead of building a broken model', () => {
    const broken = { ...spec, provider: 'bedrock' } as unknown as typeof spec;
    expect(() => createModels().model('p1', broken, 0)).toThrow('models: unknown provider bedrock');
  });
});
