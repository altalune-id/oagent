import { OpenAIModel } from '@strands-agents/sdk/models/openai';
import { describe, expect, it } from 'vitest';

import { createModels } from './index.ts';

const spec = { baseUrl: 'https://llm.example.com/v1', apiKey: 'sk-test', modelId: 'gpt-4o' };

describe('createModels', () => {
  it('builds an OpenAI-compatible chat model for the spec', () => {
    const model = createModels().model('p1', spec, 0.3);
    expect(model).toBeInstanceOf(OpenAIModel);
    expect(model.getConfig()).toMatchObject({ modelId: 'gpt-4o', temperature: 0.3 });
  });
});
