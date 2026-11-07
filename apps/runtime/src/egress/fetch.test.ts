import { describe, expect, it } from 'vitest';

import { guardedFetch, redact } from './fetch.ts';

type Call = { url: string, headers: Headers };

function fake(respond: (url: string) => Response): { fetch: typeof fetch, calls: Call[] } {
  const calls: Call[] = [];
  const impl = (async (input: string | URL, init?: RequestInit) => {
    calls.push({ url: String(input), headers: new Headers(init?.headers) });
    return respond(String(input));
  }) as typeof fetch;
  return { fetch: impl, calls };
}

const cred = { header: 'authorization', value: 'Bearer tok-test', secrets: ['Bearer tok-test', 'tok-test'] };

describe('guardedFetch', () => {
  it('drops the session id and the credential on a cross-origin hop, keeps them on the origin', async () => {
    const f = fake(url => (url === 'https://a.example.com/mcp'
      ? new Response(null, { status: 307, headers: { location: 'https://b.example.com/mcp' } })
      : new Response('ok')));
    await guardedFetch('https://a.example.com/mcp', cred, f.fetch)('https://a.example.com/mcp', { method: 'POST', body: '{}', headers: { 'mcp-session-id': 's1' } });
    expect(f.calls[0]!.headers.get('mcp-session-id')).toBe('s1');
    expect(f.calls[0]!.headers.get('authorization')).toBe('Bearer tok-test');
    expect(f.calls[1]!.headers.get('mcp-session-id')).toBeNull();
    expect(f.calls[1]!.headers.get('authorization')).toBeNull();
  });

  it('redacts the credential from an error body and caps it at 512 characters', async () => {
    const body = `{"error":"bad auth Bearer tok-test, key tok-test"}${'x'.repeat(2000)}`;
    const f = fake(() => new Response(body, { status: 401, statusText: 'Unauthorized', headers: { 'content-type': 'application/json', 'content-length': String(body.length) } }));
    const res = await guardedFetch('https://a.example.com/', cred, f.fetch)('https://a.example.com/');
    const text = await res.text();
    expect(res.status).toBe(401);
    expect(res.statusText).toBe('Unauthorized');
    expect(res.headers.get('content-type')).toBe('application/json');
    expect(text).not.toContain('tok-test');
    expect(text.startsWith('{"error":"bad auth [redacted], key [redacted]"}')).toBe(true);
    expect([...text]).toHaveLength(512);
  });

  it('reads at most 4 KiB of an error body, however long the server streams', async () => {
    let pulled = 0;
    const endless = new ReadableStream<Uint8Array>({
      pull(controller) {
        pulled += 1024;
        controller.enqueue(new Uint8Array(1024).fill(120));
      },
    });
    const f = fake(() => new Response(endless, { status: 500 }));
    const res = await guardedFetch('https://a.example.com/', cred, f.fetch)('https://a.example.com/');
    expect((await res.text()).length).toBe(512);
    expect(pulled).toBeLessThanOrEqual(8 * 1024);
  });

  it('leaves an ok body untouched', async () => {
    const f = fake(() => new Response('echo tok-test'));
    const res = await guardedFetch('https://a.example.com/', cred, f.fetch)('https://a.example.com/');
    expect(await res.text()).toBe('echo tok-test');
  });

  it('works without a credential', async () => {
    const f = fake(() => new Response('nope', { status: 500 }));
    const res = await guardedFetch('https://a.example.com/', undefined, f.fetch)('https://a.example.com/', { headers: { authorization: 'Bearer caller' } });
    expect(f.calls[0]!.headers.get('authorization')).toBeNull();
    expect(await res.text()).toBe('nope');
  });
});

describe('redact', () => {
  it('replaces the longest secret first and skips empty ones', () => {
    expect(redact('Bearer tok and tok', ['tok', 'Bearer tok', ''])).toBe('[redacted] and [redacted]');
  });
});
