import type Anthropic from '@anthropic-ai/sdk';
import { AnthropicModel } from '@strands-agents/sdk/models/anthropic';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { apiError, fakeAnthropic, messagesStream } from '../test/anthropic-api.ts';
import { fakeMcp, goldenRequest, quiet } from '../test/turn-fixtures.ts';
import { createRunTurn } from '../turn/index.ts';
import { anthropicClient, anthropicModel, supportsAdaptiveThinking } from './anthropic.ts';
import { ClientCache } from './cache.ts';
import { createModels } from './index.ts';

const spec = { provider: 'anthropic' as const, baseUrl: 'https://api.anthropic.com', apiKey: 'sk-test', modelId: 'claude-sonnet-5-5', maxTokens: 1024 };
const BINDING_ERROR = 'messages.1.content.0: thinking block is bound to a different conversation';

type Event = Record<string, unknown>;

async function runTurnEvents(modelId?: string): Promise<Event[]> {
  const events: Event[] = [];
  const runTurn = createRunTurn({ models: createModels(), log: quiet, openMcp: fakeMcp([]) });
  const request = goldenRequest('anthropic_reply', (r) => {
    if (modelId) {
      r.agent.model.modelId = modelId;
    }
  });
  await runTurn(request, async (e) => {
    events.push(e);
  }, new AbortController().signal);
  return events;
}

async function runOneTurn(modelId?: string): Promise<Event> {
  return (await runTurnEvents(modelId)).find(e => e.type === 'OAGENT_TERMINAL')!;
}

afterEach(() => {
  vi.unstubAllGlobals();
  vi.unstubAllEnvs();
  vi.restoreAllMocks();
});

describe('anthropicClient', () => {
  it('reuses one client for the same project and URL whatever the model', () => {
    const cache = new ClientCache<Anthropic>();
    const a = anthropicClient(cache, 'p1', spec);
    expect(anthropicClient(cache, 'p1', { ...spec, modelId: 'claude-opus-5-5' })).toBe(a);
    expect(cache.size).toBe(1);
  });

  it('gives another project, key or URL its own client and keys by project and URL only', () => {
    const cache = new ClientCache<Anthropic>();
    const a = anthropicClient(cache, 'p1', spec);
    const others = [
      anthropicClient(cache, 'p2', spec),
      anthropicClient(cache, 'p1', { ...spec, apiKey: 'sk-other' }),
      anthropicClient(cache, 'p1', { ...spec, baseUrl: 'https://proxy.example.com' }),
    ];
    for (const c of others) {
      expect(c).not.toBe(a);
    }
    expect(cache.keys()).toEqual(['p2:https://api.anthropic.com', 'p1:https://api.anthropic.com', 'p1:https://proxy.example.com']);
  });

  it('builds the client from the spec alone, with no retries, no bearer token, no logs and the guarded fetch', () => {
    const client = anthropicClient(new ClientCache<Anthropic>(), 'p1', spec);
    expect(client.baseURL).toBe('https://api.anthropic.com');
    expect(client.apiKey).toBe('sk-test');
    expect(client.authToken).toBeNull();
    expect(client.maxRetries).toBe(0);
    expect(client.logLevel).toBe('off');
    expect((client as unknown as { fetch: unknown }).fetch).not.toBe(globalThis.fetch);
  });
});

describe('supportsAdaptiveThinking', () => {
  it.each([
    ['claude-opus-5-5', true],
    ['claude-sonnet-5-5', true],
    ['claude-haiku-5-5', true],
    ['claude-fable-5-1', true],
    ['claude-opus-5', true],
    ['claude-opus-4-8', true],
    ['claude-opus-4-6', true],
    ['claude-sonnet-4-6', true],
    ['claude-opus-6', true],
    ['claude-haiku-4-5', false],
    ['claude-sonnet-4-5', false],
    ['claude-sonnet-4-5-20250929', false],
    ['claude-opus-4-1', false],
    ['claude-opus-4-20250514', false],
    ['claude-3-5-sonnet-20241022', false],
    ['claude-3-opus-latest', false],
    ['claude-opus-4-6@20260205', true],
    ['claude-sonnet-4-6-20260217', true],
    ['anthropic.claude-sonnet-5-5-v1:0', true],
    ['us.anthropic.claude-opus-5-5', true],
    ['my-proxy-alias', true],
    ['claude-opus-4-5@20251101', false],
    ['anthropic.claude-haiku-4-5-20251001-v1:0', false],
    ['claude-sonnet-4-20250514', false],
    ['claude-3-7-sonnet-latest', false],
    ['claude-2.1', false],
    ['claude-instant-1.2', false],
  ])('%s gets adaptive thinking: %s', (modelId, expected) => {
    expect(supportsAdaptiveThinking(modelId)).toBe(expected);
  });
});

describe('anthropicModel', () => {
  it('turns on adaptive thinking and Anthropic caching and never sets a temperature', () => {
    const model = anthropicModel(new ClientCache<Anthropic>(), 'p1', spec);
    expect(model).toBeInstanceOf(AnthropicModel);
    const config = model.getConfig();
    expect(config).toMatchObject({ modelId: 'claude-sonnet-5-5', maxTokens: 1024, cacheConfig: { strategy: 'anthropic' }, params: { thinking: { type: 'adaptive' } } });
    expect(config).not.toHaveProperty('temperature');
  });

  it('sends no thinking params to a model without adaptive thinking', () => {
    const config = anthropicModel(new ClientCache<Anthropic>(), 'p1', { ...spec, modelId: 'claude-haiku-4-5' }).getConfig();
    expect(config).not.toHaveProperty('params');
  });

  it('leaves maxTokens to the Strands default when the request has none', () => {
    const { maxTokens: _drop, ...noMax } = spec;
    const cache = new ClientCache<Anthropic>();
    const strandsDefault = new AnthropicModel({ client: anthropicClient(cache, 'p1', noMax), modelId: noMax.modelId }).getConfig().maxTokens;
    expect(anthropicModel(cache, 'p1', noMax).getConfig().maxTokens).toBe(strandsDefault);
  });
});

describe('an Anthropic turn on the wire', () => {
  it('posts to <baseUrl>/v1/messages with x-api-key only, adaptive thinking, cache points and no temperature', async () => {
    const api = fakeAnthropic([() => messagesStream('Hi, how can I help?')]);
    vi.stubGlobal('fetch', api.fetch);

    const terminal = await runOneTurn();

    expect(api.seen).toHaveLength(1);
    const [req] = api.seen;
    expect(req!.url).toBe('https://api.anthropic.com/v1/messages');
    expect(req!.headers.get('x-api-key')).toBe('sk-test');
    expect(req!.headers.has('authorization')).toBe(false);
    expect(req!.redirect).toBe('manual');
    expect(req!.body.thinking).toEqual({ type: 'adaptive' });
    expect(req!.body).not.toHaveProperty('temperature');
    expect(JSON.stringify(req!.body)).toContain('"cache_control"');
    expect(terminal.status).toBe('completed');
    expect(terminal.usage).toMatchObject({ cacheReadInputTokens: 80, cacheWriteInputTokens: 40 });
  });

  it('sends no thinking field to an older model', async () => {
    const api = fakeAnthropic([() => messagesStream('Hi, how can I help?')]);
    vi.stubGlobal('fetch', api.fetch);

    const terminal = await runOneTurn('claude-haiku-4-5');

    expect(api.seen[0]!.body.model).toBe('claude-haiku-4-5');
    expect(api.seen[0]!.body).not.toHaveProperty('thinking');
    expect(terminal.status).toBe('completed');
  });

  it('fails a refused turn with model_error, no snapshot and no refused message', async () => {
    const api = fakeAnthropic([() => messagesStream('I can\'t help with', 'refusal')]);
    vi.stubGlobal('fetch', api.fetch);

    const events = await runTurnEvents();

    const terminal = events.find(e => e.type === 'OAGENT_TERMINAL')!;
    expect(terminal).toMatchObject({ status: 'failed', stopReason: 'refusal', snapshot: null, error: { code: 'model_error' } });
    expect(terminal.usage).toMatchObject({ outputTokens: 9, cacheReadInputTokens: 80, cacheWriteInputTokens: 40, estimated: false });
    expect((terminal.messagesAppended as Event[]).map(m => m.role)).toEqual(['user']);
    expect(events.find(e => e.type === 'RUN_ERROR')).toMatchObject({ code: 'model_error' });
    expect(events.some(e => e.type === 'RUN_FINISHED')).toBe(false);
  });

  it('retries once with drop_block and the binding beta when a thinking block is bound to another conversation', async () => {
    const api = fakeAnthropic([() => apiError(400, BINDING_ERROR), () => messagesStream('Hi again.')]);
    vi.stubGlobal('fetch', api.fetch);

    const terminal = await runOneTurn();

    expect(api.seen).toHaveLength(2);
    expect(api.seen[0]!.body.thinking).toEqual({ type: 'adaptive' });
    expect(api.seen[1]!.body.thinking).toEqual({ type: 'adaptive', block_binding: { prefix_mismatch_behavior: 'drop_block' } });
    expect(api.seen[1]!.headers.get('anthropic-beta')).toContain('thinking-binding-controls-2026-08-01');
    expect(terminal.status).toBe('completed');
  });

  it('ignores the ANTHROPIC_* environment: same URL and headers, and no SDK logs', async () => {
    vi.stubEnv('ANTHROPIC_API_KEY', 'sk-env');
    vi.stubEnv('ANTHROPIC_BASE_URL', 'https://env.example.com');
    vi.stubEnv('ANTHROPIC_AUTH_TOKEN', 'tok-env');
    vi.stubEnv('ANTHROPIC_LOG', 'debug');
    const printed: unknown[] = [];
    for (const level of ['debug', 'info', 'log', 'warn', 'error'] as const) {
      vi.spyOn(console, level).mockImplementation((...args: unknown[]) => {
        printed.push(...args);
      });
    }
    const api = fakeAnthropic([() => messagesStream('Hi, how can I help?')]);
    vi.stubGlobal('fetch', api.fetch);

    await runOneTurn();

    const client = anthropicClient(new ClientCache<Anthropic>(), 'p1', spec);
    expect([client.apiKey, client.authToken, client.baseURL]).toEqual(['sk-test', null, 'https://api.anthropic.com']);
    const [req] = api.seen;
    expect(req!.url).toBe('https://api.anthropic.com/v1/messages');
    expect(req!.headers.get('x-api-key')).toBe('sk-test');
    expect(req!.headers.has('authorization')).toBe(false);
    expect(printed).toEqual([]);
  });
});
