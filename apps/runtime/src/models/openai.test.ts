import type OpenAI from 'openai';
import { OpenAIModel } from '@strands-agents/sdk/models/openai';
import { describe, expect, it } from 'vitest';

import { ClientCache } from './cache.ts';
import { openaiClient, openaiModel } from './openai.ts';

const spec = { provider: 'openai_compatible' as const, baseUrl: 'https://llm.example.com/v1', apiKey: 'sk-test', modelId: 'gpt-4o', maxTokens: 1024 };

describe('openaiClient', () => {
  it('reuses one client for the same project and URL whatever the model', () => {
    const cache = new ClientCache<OpenAI>();
    const a = openaiClient(cache, 'p1', spec);
    expect(openaiClient(cache, 'p1', { ...spec, modelId: 'gpt-4o-mini', maxTokens: 10 })).toBe(a);
    expect(cache.size).toBe(1);
  });

  it('gives another project, key or URL its own client', () => {
    const cache = new ClientCache<OpenAI>();
    const a = openaiClient(cache, 'p1', spec);
    const others = [
      openaiClient(cache, 'p2', spec),
      openaiClient(cache, 'p1', { ...spec, apiKey: 'sk-other' }),
      openaiClient(cache, 'p1', { ...spec, baseUrl: 'https://other.example.com/v1' }),
    ];
    for (const c of others) {
      expect(c).not.toBe(a);
    }
    expect(new Set(others).size).toBe(3);
  });

  it('keys by project and URL, never the API key', () => {
    const cache = new ClientCache<OpenAI>();
    openaiClient(cache, 'p1', spec);
    expect(cache.keys()).toEqual(['p1:https://llm.example.com/v1']);
  });

  it('builds the client for the configured URL and key', () => {
    const client = openaiClient(new ClientCache<OpenAI>(), 'p1', spec);
    expect(client.baseURL).toBe('https://llm.example.com/v1');
    expect(client.apiKey).toBe('sk-test');
    expect(client.maxRetries).toBe(0);
    expect(client.organization).toBeNull();
    expect(client.project).toBeNull();
    expect((client as unknown as { fetch: unknown }).fetch).not.toBe(globalThis.fetch);
  });
});

describe('openaiModel', () => {
  it('makes a new chat model per turn over the cached client', () => {
    const cache = new ClientCache<OpenAI>();
    const m1 = openaiModel(cache, 'p1', spec, 0.2);
    const m2 = openaiModel(cache, 'p1', { ...spec, modelId: 'gpt-4o-mini' }, 0.7);
    expect(m1).toBeInstanceOf(OpenAIModel);
    expect(m1).not.toBe(m2);
    expect(m1.api).toBe('chat');
    expect(m1.modelId).toBe('gpt-4o');
    expect(m1.getConfig()).toMatchObject({ temperature: 0.2, maxTokens: 1024 });
    expect(m2.getConfig()).toMatchObject({ modelId: 'gpt-4o-mini', temperature: 0.7 });
    expect(m2.getConfig().maxTokens).toBe(1024);
    expect(cache.size).toBe(1);
  });

  it('leaves maxTokens unset when the request has none', () => {
    const { maxTokens: _drop, ...noMax } = spec;
    expect(openaiModel(new ClientCache<OpenAI>(), 'p1', noMax, 0).getConfig().maxTokens).toBeUndefined();
  });
});
