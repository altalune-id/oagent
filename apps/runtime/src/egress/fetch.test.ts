import { describe, expect, it } from 'vitest';

import { guardedFetch, readCapped, redact } from './fetch.ts';

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

  it('never returns a secret that straddles the 4 KiB error read', async () => {
    const f = fake(() => new Response(`${'x'.repeat(4090)}tok-test`, { status: 500 }));
    expect(await (await guardedFetch('https://a.example.com/', cred, f.fetch)('https://a.example.com/')).text()).toBe('x'.repeat(512));
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

describe('guardedFetch options', () => {
  const refuse = (url: URL) => {
    if (url.hostname !== 'a.example.com') {
      throw new Error(`refused ${url.hostname}`);
    }
  };
  const hostCredentials = new Map([['a.example.com', [{ header: 'x-key', value: 'key-a' }]]]);

  it('checks the first URL before any request', async () => {
    const f = fake(() => new Response('ok'));
    await expect(guardedFetch('https://b.example.com/', undefined, f.fetch, { checkHop: refuse })('https://b.example.com/')).rejects.toThrow('refused b.example.com');
    expect(f.calls).toHaveLength(0);
  });

  it('checks every redirect hop and stops at the refused one', async () => {
    const f = fake(() => new Response(null, { status: 302, headers: { location: 'https://b.example.com/next' } }));
    await expect(guardedFetch('https://a.example.com/', undefined, f.fetch, { checkHop: refuse })('https://a.example.com/')).rejects.toThrow('refused b.example.com');
    expect(f.calls).toHaveLength(1);
  });

  it('returns the redirect itself when following is off', async () => {
    const f = fake(() => new Response(null, { status: 302, headers: { location: 'https://a.example.com/next' } }));
    const res = await guardedFetch('https://a.example.com/', undefined, f.fetch, { followRedirects: false })('https://a.example.com/', { method: 'POST', body: '{}' });
    expect(res.status).toBe(302);
    expect(f.calls).toHaveLength(1);
  });

  it('sends a host secret only to its host, never across a redirect or to another first host', async () => {
    const f = fake(url => (url === 'https://a.example.com/' ? new Response(null, { status: 307, headers: { location: 'https://b.example.com/' } }) : new Response('ok')));
    const send = guardedFetch('https://a.example.com/', undefined, f.fetch, { hostCredentials });
    await send('https://a.example.com/', { headers: { 'x-key': 'model-value' } });
    await send('https://c.example.com/', { headers: { 'x-key': 'model-value' } });
    expect(f.calls.map(c => [new URL(c.url).hostname, c.headers.get('x-key')])).toEqual([
      ['a.example.com', 'key-a'],
      ['b.example.com', 'model-value'],
      ['c.example.com', 'model-value'],
    ]);
  });

  it('redacts a host secret from an error body', async () => {
    const f = fake(() => new Response('bad key-a', { status: 403 }));
    const res = await guardedFetch('https://a.example.com/', undefined, f.fetch, { hostCredentials })('https://a.example.com/');
    expect(await res.text()).toBe('bad [redacted]');
  });
});

describe('readCapped', () => {
  const chunks = (n: number, size: number) => new Response(new ReadableStream<Uint8Array>({
    start(controller) {
      for (let i = 0; i < n; i++) {
        controller.enqueue(new Uint8Array(size).fill(97));
      }
      controller.close();
    },
  }));

  it('reports no truncation for a body of exactly maxBytes', async () => {
    const out = await readCapped(chunks(4, 256), 1024);
    expect([out.bytes.length, out.truncated]).toEqual([1024, false]);
  });

  it('cuts at maxBytes and reports truncation one byte past it', async () => {
    const out = await readCapped(new Response('a'.repeat(1025)), 1024);
    expect([out.bytes.length, out.truncated]).toEqual([1024, true]);
  });

  it('cancels the stream once the cap is passed', async () => {
    let cancelled = 0;
    const endless = new Response(new ReadableStream<Uint8Array>({
      pull(controller) {
        controller.enqueue(new Uint8Array(512));
      },
      cancel() {
        cancelled++;
      },
    }));
    expect((await readCapped(endless, 1024)).truncated).toBe(true);
    expect(cancelled).toBe(1);
  });

  it('reads an empty body', async () => {
    expect(await readCapped(new Response(null), 10)).toEqual({ bytes: new Uint8Array(0), truncated: false });
  });
});

describe('redact', () => {
  it('replaces the longest secret first and skips empty ones', () => {
    expect(redact('Bearer tok and tok', ['tok', 'Bearer tok', ''])).toBe('[redacted] and [redacted]');
  });
});
