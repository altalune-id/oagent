import { OpenAIModel } from '@strands-agents/sdk/models/openai';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { ModelClients } from './models.ts';

const spec = { baseUrl: 'https://llm.example.com/v1', apiKey: 'sk-test', modelId: 'gpt-4o', maxTokens: 1024 };

afterEach(() => {
  vi.useRealTimers();
});

describe('modelClients', () => {
  it('reuses one client for the same project and config', () => {
    const clients = new ModelClients();
    const a = clients.client('p1', spec);
    expect(clients.client('p1', spec)).toBe(a);
    expect(clients.client('p1', { ...spec, modelId: 'gpt-4o-mini', maxTokens: 10 })).toBe(a);
    expect(clients.size).toBe(1);
  });

  it('gives another project, key or URL its own client', () => {
    const clients = new ModelClients();
    const a = clients.client('p1', spec);
    const others = [
      clients.client('p2', spec),
      clients.client('p1', { ...spec, apiKey: 'sk-other' }),
      clients.client('p1', { ...spec, baseUrl: 'https://other.example.com/v1' }),
    ];
    for (const c of others) {
      expect(c).not.toBe(a);
    }
    expect(new Set(others).size).toBe(3);
    expect(clients.size).toBe(3);
  });

  it('replaces the client when the key changes for the same URL', () => {
    const clients = new ModelClients();
    const old = clients.client('p1', spec);
    const fresh = clients.client('p1', { ...spec, apiKey: 'sk-rotated' });
    expect(fresh).not.toBe(old);
    expect(fresh.apiKey).toBe('sk-rotated');
    expect(clients.size).toBe(1);
    expect(clients.client('p1', { ...spec, apiKey: 'sk-rotated' })).toBe(fresh);
    expect(clients.client('p1', spec)).not.toBe(old);
  });

  it('keys by project and URL, never the API key', () => {
    const clients = new ModelClients();
    clients.client('p1', spec);
    expect(clients.keys()).toEqual(['p1:https://llm.example.com/v1']);
  });

  it('builds the client for the configured URL and key', () => {
    const client = new ModelClients().client('p1', spec);
    expect(client.baseURL).toBe('https://llm.example.com/v1');
    expect(client.apiKey).toBe('sk-test');
    expect(client.maxRetries).toBe(0);
    expect(client.organization).toBeNull();
    expect(client.project).toBeNull();
    expect((client as unknown as { fetch: unknown }).fetch).not.toBe(globalThis.fetch);
  });

  it('evicts the least recently used past the bound', () => {
    const clients = new ModelClients({ max: 2 });
    const a = clients.client('a', spec);
    const b = clients.client('b', spec);
    expect(clients.client('a', spec)).toBe(a);
    clients.client('c', spec);
    expect(clients.size).toBe(2);
    expect(clients.client('a', spec)).toBe(a);
    expect(clients.client('b', spec)).not.toBe(b);
  });

  it('drops a client idle for longer than the expiry', () => {
    vi.useFakeTimers();
    const clients = new ModelClients({ idleMs: 15 * 60_000 });
    const a = clients.client('p1', spec);
    vi.advanceTimersByTime(15 * 60_000);
    expect(clients.client('p1', spec)).toBe(a);
    vi.advanceTimersByTime(15 * 60_000 + 1);
    const b = clients.client('p1', spec);
    expect(b).not.toBe(a);
    clients.client('p2', spec);
    vi.advanceTimersByTime(15 * 60_000 + 1);
    clients.client('p3', spec);
    expect(clients.keys().map(k => k.split(':')[0])).toEqual(['p3']);
  });

  it('defaults to 256 entries and a 15 minute expiry', () => {
    const clients = new ModelClients();
    expect(clients.max).toBe(256);
    expect(clients.idleMs).toBe(15 * 60_000);
  });

  it('makes a new chat model per turn over the cached client', () => {
    const clients = new ModelClients();
    const m1 = clients.model('p1', spec, 0.2);
    const m2 = clients.model('p1', { ...spec, modelId: 'gpt-4o-mini' }, 0.7);
    expect(m1).toBeInstanceOf(OpenAIModel);
    expect(m1).not.toBe(m2);
    expect(m1.api).toBe('chat');
    expect(m1.modelId).toBe('gpt-4o');
    expect(m1.getConfig()).toMatchObject({ temperature: 0.2, maxTokens: 1024 });
    expect(m2.getConfig()).toMatchObject({ modelId: 'gpt-4o-mini', temperature: 0.7 });
    expect(m2.getConfig().maxTokens).toBe(1024);
    expect(clients.size).toBe(1);
  });

  it('leaves maxTokens unset when the request has none', () => {
    const { maxTokens: _drop, ...noMax } = spec;
    const model = new ModelClients().model('p1', noMax, 0);
    expect(model.getConfig().maxTokens).toBeUndefined();
  });
});
