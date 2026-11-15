import { afterEach, describe, expect, it, vi } from 'vitest';

import { secretForms } from '../../egress/index.ts';
import { failed, stubFetch, webTurn } from '../../test/web.ts';
import { httpTool } from './http.ts';

const spec = {
  hosts: ['api.example.com', 'b.example.com'],
  requestHeaders: ['Accept', 'X-Trace', 'Cookie', 'Authorization'],
  secrets: [{ host: 'api.example.com', header: 'Authorization', value: 'Bearer abc123' }],
};

type Output = { status: number, headers: Record<string, string>, body: string, truncated?: boolean };

function parse(out: unknown): Output {
  const inner = String(out).split('\n').slice(1, -1).join('\n').replace(/&lt;/g, '<').replace(/&gt;/g, '>').replace(/&amp;/g, '&');
  return JSON.parse(inner) as Output;
}

function setup(over: Parameters<typeof webTurn>[0] = {}) {
  const turn = webTurn({ secrets: secretForms(spec.secrets.map(s => s.value)), ...over });
  return { turn, tool: httpTool(turn, spec) };
}

describe('http_request', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.useRealTimers();
  });

  it('declares method, url, headers, body and timeout_ms in that order', () => {
    expect(Object.keys((setup().tool.toolSpec.inputSchema as { properties: object }).properties)).toEqual(['method', 'url', 'headers', 'body', 'timeout_ms']);
  });

  it('follows a checked GET redirect and attaches the secret only on its host', async () => {
    const sent = stubFetch(url => (url.startsWith('https://api.example.com/') ? new Response(null, { status: 302, headers: { location: 'https://b.example.com/x' } }) : new Response('{"ok":true}', { headers: { 'content-type': 'application/json' } })));
    const out = parse(await setup().tool.invoke({ method: 'GET', url: 'https://api.example.com/v1' }));
    expect(out).toEqual({ status: 200, headers: { 'content-type': 'application/json' }, body: '{"ok":true}' });
    expect(sent.map(s => [new URL(s.url).hostname, s.headers.get('authorization')])).toEqual([['api.example.com', 'Bearer abc123'], ['b.example.com', null]]);
  });

  it('refuses an unlisted host or a redirect to one', async () => {
    const sent = stubFetch(() => new Response(null, { status: 302, headers: { location: 'https://evil.example/' } }));
    const { tool } = setup();
    expect((await failed(tool.invoke({ method: 'GET', url: 'https://evil.example/' }))).message).toBe('http_request failed: blocked_host (host evil.example)');
    expect((await failed(tool.invoke({ method: 'GET', url: 'https://api.example.com/' }))).message).toBe('http_request failed: blocked_host (host evil.example)');
    expect(sent).toHaveLength(1);
  });

  it('never follows a redirect on a non-GET and returns the 3xx', async () => {
    const sent = stubFetch(() => new Response(null, { status: 302, headers: { location: 'https://b.example.com/done' } }));
    const out = parse(await setup().tool.invoke({ method: 'POST', url: 'https://api.example.com/v1', body: '{}' }));
    expect([out.status, out.headers.location, sent.length]).toEqual([302, 'https://b.example.com/done', 1]);
  });

  it('sends only listed, unreserved model headers; the secret overwrites a model value', async () => {
    const sent = stubFetch(() => new Response('ok'));
    await setup().tool.invoke({ method: 'GET', url: 'https://api.example.com/', headers: { 'Accept': 'text/plain', 'X-Trace': 't1', 'Host': 'evil', 'Cookie': 'c=1', 'X-Forwarded-For': '1.2.3.4', 'X-Other': 'o', 'Authorization': 'Bearer model' } });
    await setup().tool.invoke({ method: 'GET', url: 'https://b.example.com/', headers: { Authorization: 'Bearer model' } });
    const first = sent[0]!.headers;
    expect([first.get('accept'), first.get('x-trace'), first.get('host'), first.get('cookie'), first.get('x-forwarded-for'), first.get('x-other'), first.get('authorization')]).toEqual(['text/plain', 't1', null, null, null, null, 'Bearer abc123']);
    expect(sent[1]!.headers.get('authorization')).toBeNull();
  });

  it('returns only allow-listed response headers, a location only when it passes policy, never set-cookie', async () => {
    stubFetch(() => new Response('x', { headers: { 'content-type': 'text/plain', 'etag': '"e1"', 'set-cookie': 's=1', 'x-internal': 'i' } }));
    expect(parse(await setup().tool.invoke({ method: 'GET', url: 'https://api.example.com/' })).headers).toEqual({ 'content-type': 'text/plain', 'etag': '"e1"' });
    stubFetch(() => new Response(null, { status: 303, headers: { location: 'https://evil.example/?leak=1' } }));
    expect('location' in parse(await setup().tool.invoke({ method: 'DELETE', url: 'https://api.example.com/' })).headers).toBe(false);
    stubFetch(() => new Response(null, { status: 307, headers: { location: '/relative' } }));
    expect(parse(await setup().tool.invoke({ method: 'PUT', url: 'https://api.example.com/a/b', body: 'x' })).headers.location).toBe('https://api.example.com/relative');
  });

  it('redacts a secret echoed in the body or a header, including the bare token', async () => {
    stubFetch(() => new Response('your token abc123 and Bearer abc123', { headers: { 'content-type': 'text/plain', 'etag': 'abc123' } }));
    const out = await setup().tool.invoke({ method: 'GET', url: 'https://api.example.com/' }) as string;
    expect(out).not.toContain('abc123');
    expect(parse(out)).toEqual({ status: 200, headers: { 'content-type': 'text/plain', 'etag': '[redacted]' }, body: 'your token [redacted] and [redacted]' });
  });

  it('redacts before cutting, so a secret across the 20,000 cut leaves no prefix', async () => {
    const tool = httpTool(webTurn({ secrets: secretForms(['Bearer Zq9Wk7Lp']) }), { ...spec, secrets: [{ host: 'api.example.com', header: 'Authorization', value: 'Bearer Zq9Wk7Lp' }] });
    stubFetch(() => new Response(`${'x'.repeat(19_996)}Zq9Wk7Lp tail`, { headers: { 'content-type': 'text/plain' } }));
    const out = await tool.invoke({ method: 'GET', url: 'https://api.example.com/' }) as string;
    expect(out).not.toContain('Zq9');
  });

  it('redacts a bare token across the 512 cut of an error body', async () => {
    const tool = httpTool(webTurn({ secrets: secretForms(['Bearer Zq9Wk7Lp']) }), { ...spec, secrets: [{ host: 'api.example.com', header: 'Authorization', value: 'Bearer Zq9Wk7Lp' }] });
    stubFetch(() => new Response(`${'e'.repeat(508)}Zq9Wk7Lp more`, { status: 400 }));
    const out = await tool.invoke({ method: 'POST', url: 'https://api.example.com/', body: '{}' }) as string;
    expect(out).not.toContain('Zq9');
  });

  it('redacts a secret with a quote or backslash before JSON encoding', async () => {
    const value = 'Bearer a"b\\c-Zq9';
    const tool = httpTool(webTurn({ secrets: secretForms([value]) }), { ...spec, secrets: [{ host: 'api.example.com', header: 'Authorization', value }] });
    stubFetch(() => new Response(`echo ${value}`, { headers: { 'content-type': 'text/plain', 'etag': value } }));
    const out = await tool.invoke({ method: 'GET', url: 'https://api.example.com/' }) as string;
    expect(out).not.toContain('Zq9');
    expect(parse(out).body).toBe('echo [redacted]');
  });

  it('cuts the text at 20,000 characters, reads at most 1 MiB and marks truncation', async () => {
    stubFetch(() => new Response('a'.repeat(25_000), { headers: { 'content-type': 'text/plain' } }));
    const out = parse(await setup().tool.invoke({ method: 'GET', url: 'https://api.example.com/' }));
    expect([out.body.length, out.truncated]).toEqual([20_000, true]);
    let pulled = 0;
    stubFetch(() => new Response(new ReadableStream<Uint8Array>({
      pull(controller) {
        pulled += 65536;
        controller.enqueue(new Uint8Array(65536).fill(98));
      },
    }), { headers: { 'content-type': 'application/octet-stream' } }));
    const binary = parse(await setup().tool.invoke({ method: 'GET', url: 'https://api.example.com/' }));
    expect([binary.body, binary.truncated]).toEqual(['[binary body: 1048576 bytes]', true]);
    expect(pulled).toBeLessThanOrEqual(1024 * 1024 + 2 * 65536);
    stubFetch(() => new Response('short', { headers: { 'content-type': 'text/plain' } }));
    expect('truncated' in parse(await setup().tool.invoke({ method: 'GET', url: 'https://api.example.com/' }))).toBe(false);
  });

  it('returns a 4xx body capped at 512 characters and marks it truncated', async () => {
    stubFetch(() => new Response(`{"error":"${'e'.repeat(2000)}"}`, { status: 422, headers: { 'content-type': 'application/json' } }));
    const out = parse(await setup().tool.invoke({ method: 'POST', url: 'https://api.example.com/', body: '{}' }));
    expect([out.status, out.body.length, out.truncated]).toEqual([422, 512, true]);
    stubFetch(() => new Response('{"error":"bad"}', { status: 400 }));
    const short = parse(await setup().tool.invoke({ method: 'POST', url: 'https://api.example.com/', body: '{}' }));
    expect([short.status, short.body, 'truncated' in short]).toEqual([400, '{"error":"bad"}', false]);
  });

  it('refuses a body on GET and a body over 64 KiB before sending', async () => {
    const sent = stubFetch(() => new Response('ok'));
    const { tool } = setup();
    expect((await failed(tool.invoke({ method: 'GET', url: 'https://api.example.com/', body: 'x' }))).message).toBe('http_request failed: invalid_request (host api.example.com)');
    expect((await failed(tool.invoke({ method: 'POST', url: 'https://api.example.com/', body: 'é'.repeat(32 * 1024 + 1) }))).message).toBe('http_request failed: invalid_request (host api.example.com)');
    expect((await tool.invoke({ method: 'POST', url: 'https://api.example.com/', body: 'x'.repeat(64 * 1024) }) as string).length).toBeGreaterThan(0);
    expect(sent).toHaveLength(1);
  });

  it('lets timeout_ms lower the timeout but never raise it', async () => {
    const hang = () => stubFetch((_url, init) => new Promise<Response>((_resolve, reject) => {
      init.signal?.addEventListener('abort', () => reject(new Error('aborted', { cause: init.signal?.reason })));
    }));
    hang();
    const started = Date.now();
    await failed(setup({ timeoutMs: 100 }).tool.invoke({ method: 'GET', url: 'https://api.example.com/', timeout_ms: 30_000 }));
    expect(Date.now() - started).toBeLessThan(2000);
    hang();
    const lowered = Date.now();
    const error = await failed(setup({ timeoutMs: 30_000 }).tool.invoke({ method: 'GET', url: 'https://api.example.com/', timeout_ms: 1000 }));
    expect([error.message, Date.now() - lowered < 5000]).toEqual(['http_request failed: timeout', true]);
  });
});
