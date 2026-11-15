import type { WebToolError } from '../errors.ts';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { failed, json, stubFetch } from '../../../test/web.ts';
import { exa } from './exa.ts';

const KEY = 'exa-secret-key-456';
const signal = new AbortController().signal;

describe('exa', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('posts an auto search for 5 results with highlights and the key header', async () => {
    const sent = stubFetch(() => json({ results: [] }));
    await exa.search('oagent docs', KEY, signal);
    expect([sent[0]!.url, sent[0]!.method, sent[0]!.headers.get('x-api-key'), sent[0]!.headers.get('content-type')]).toEqual(['https://api.exa.ai/search', 'POST', KEY, 'application/json']);
    expect(JSON.parse(sent[0]!.body!)).toEqual({ query: 'oagent docs', type: 'auto', numResults: 5, contents: { highlights: true } });
  });

  it('takes the first highlight, then summary, then text as the snippet', async () => {
    stubFetch(() => json({ results: [
      { title: 'A', url: 'https://a.example.com/', publishedDate: '2026-03-01T00:00:00.000Z', highlights: ['first', 'second'], summary: 's' },
      { title: 'B', url: 'https://b.example.com/', summary: 'sum', text: 'txt' },
      { title: 'C', url: 'https://c.example.com/', text: 'x'.repeat(500) },
      { title: 'D', url: 'not a url' },
    ] }));
    const out = await exa.search('q', KEY, signal);
    expect(out.map(r => r.snippet.slice(0, 5))).toEqual(['first', 'sum', 'xxxxx']);
    expect(out[0]!.publishedAt).toBe('2026-03-01T00:00:00.000Z');
    expect(out[2]!.snippet).toHaveLength(300);
  });

  it.each([
    [401, 'bad_key'],
    [402, 'quota'],
    [403, 'invalid_request'],
    [429, 'rate_limited'],
    [400, 'invalid_request'],
    [500, 'upstream_error'],
  ])('maps status %i to %s', async (status, cls) => {
    stubFetch(() => json({ error: `bad ${KEY}`, tag: 'X' }, status));
    const error = await failed<WebToolError>(exa.search('q', KEY, signal));
    expect([error.class, error.status, error.host, error.billed]).toEqual([cls, status, 'api.exa.ai', false]);
    expect(error.message).not.toContain(KEY);
  });

  it('never follows a redirect', async () => {
    const sent = stubFetch(() => new Response(null, { status: 307, headers: { location: 'https://api.exa.ai/v2/search' } }));
    const error = await failed<WebToolError>(exa.search('q', KEY, signal));
    expect([sent.length, error.class, error.status]).toEqual([1, 'upstream_error', 307]);
  });

  it('marks a 2xx with no results array as billed', async () => {
    stubFetch(() => json({ nope: true }));
    const error = await failed<WebToolError>(exa.search('q', KEY, signal));
    expect([error.class, error.billed]).toEqual(['upstream_error', true]);
  });
});
