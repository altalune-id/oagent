import type { WebToolError } from '../errors.ts';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { failed, json, stubFetch } from '../../../test/web.ts';
import { brave } from './brave.ts';

const KEY = 'brave-secret-key-123';
const signal = new AbortController().signal;

async function failure(respond: () => Response): Promise<WebToolError> {
  stubFetch(respond);
  return failed<WebToolError>(brave.search('q', KEY, signal));
}

describe('brave', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('sends a GET with the key header to the fixed endpoint and asks for 5 results', async () => {
    const sent = stubFetch(() => json({ web: { results: [] } }));
    await brave.search('oagent docs', KEY, signal);
    expect(sent).toHaveLength(1);
    const url = new URL(sent[0]!.url);
    expect([url.origin + url.pathname, url.searchParams.get('q'), url.searchParams.get('count'), sent[0]!.method]).toEqual(['https://api.search.brave.com/res/v1/web/search', 'oagent docs', '5', 'GET']);
    expect(sent[0]!.headers.get('x-subscription-token')).toBe(KEY);
  });

  it('normalizes results: strips tags, decodes entities, reads page_age as UTC, drops bad urls, keeps 5', async () => {
    const results = [
      { title: 'A &amp; B', url: 'https://a.example.com/', description: 'the <strong>best</strong> &quot;docs&quot;', page_age: '2026-01-02T03:04:05' },
      { title: 'no url', url: 'javascript:alert(1)', description: 'x' },
      { title: 'bad date', url: 'https://b.example.com/', description: 'y', page_age: 'yesterday' },
      ...Array.from({ length: 6 }, (_, i) => ({ title: `t${i}`, url: `https://c${i}.example.com/`, description: 'z'.repeat(400) })),
    ];
    stubFetch(() => json({ web: { results } }));
    const out = await brave.search('q', KEY, signal);
    expect(out).toHaveLength(5);
    expect(out[0]).toEqual({ title: 'A & B', url: 'https://a.example.com/', snippet: 'the best "docs"', publishedAt: '2026-01-02T03:04:05.000Z' });
    expect(out[1]).toEqual({ title: 'bad date', url: 'https://b.example.com/', snippet: 'y' });
    expect(out[2]!.snippet).toHaveLength(300);
  });

  it.each([
    [401, {}, 'bad_key'],
    [403, {}, 'bad_key'],
    [422, { error: { code: 'SUBSCRIPTION_TOKEN_INVALID' } }, 'bad_key'],
    [422, { error: { code: 'VALIDATION' } }, 'invalid_request'],
    [429, { error: { code: 'QUOTA_LIMITED' } }, 'quota'],
    [402, { error: { code: 'QUOTA_LIMITED' } }, 'quota'],
    [403, { error: { code: 'QUOTA_LIMITED' } }, 'quota'],
    [429, { error: { code: 'RATE_LIMITED' } }, 'rate_limited'],
    [400, {}, 'invalid_request'],
    [503, {}, 'upstream_error'],
  ])('maps status %i %j to %s, never billed', async (status, body, cls) => {
    const error = await failure(() => json(body, status));
    expect([error.class, error.status, error.host, error.billed]).toEqual([cls, status, 'api.search.brave.com', false]);
  });

  it('redacts the key before cutting a snippet', async () => {
    stubFetch(() => json({ web: { results: [{ title: 't', url: 'https://a.example.com/', description: `${'d'.repeat(295)}${KEY}` }] } }));
    const [result] = await brave.search('q', KEY, signal);
    expect(result!.snippet).not.toContain('brave');
  });

  it('strips tags from hostile provider text fast', async () => {
    stubFetch(() => json({ web: { results: [{ title: '<a '.repeat(87_382), url: 'https://a.example.com/', description: '<b '.repeat(87_382) }] } }));
    const started = performance.now();
    await brave.search('q', KEY, signal);
    expect(performance.now() - started).toBeLessThan(1000);
  });

  it('never follows a redirect and never echoes the key', async () => {
    const sent = stubFetch(() => new Response(`moved ${KEY}`, { status: 302, headers: { location: 'https://evil.example/' } }));
    const error = await failed<WebToolError>(brave.search('q', KEY, signal));
    expect(sent).toHaveLength(1);
    expect(error.class).toBe('upstream_error');
    expect(error.message).not.toContain(KEY);
  });

  it('marks a 2xx it cannot parse as billed', async () => {
    const error = await failure(() => new Response('{not json', { status: 200 }));
    expect([error.class, error.billed]).toEqual(['upstream_error', true]);
  });
});
