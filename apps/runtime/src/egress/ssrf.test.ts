import type { Server } from 'node:http';
import type { AddressInfo } from 'node:net';
import { readFileSync } from 'node:fs';
import { createServer } from 'node:http';
import { fileURLToPath } from 'node:url';
import { getGlobalDispatcher } from 'undici';
import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest';

import { checkedLookup, guardedLookup, installGuard, isPrivateOrLocal, SsrfRefusedError } from './ssrf.ts';

const vectorsPath = fileURLToPath(new URL('../../../../httpclient/testdata/ssrf_vectors.json', import.meta.url));
const vectors = JSON.parse(readFileSync(vectorsPath, 'utf8')) as { private: string[], public: string[] };

function listen(handler: Parameters<typeof createServer>[1]): Promise<{ server: Server, port: number }> {
  return new Promise((resolve) => {
    const server = createServer(handler);
    server.listen(0, '127.0.0.1', () => {
      resolve({ server, port: (server.address() as AddressInfo).port });
    });
  });
}

async function refusal(url: string): Promise<unknown> {
  try {
    await fetch(url, { redirect: 'follow' });
  } catch (error) {
    return (error as Error).cause;
  }
  throw new Error(`fetch to ${url} was not refused`);
}

describe('isPrivateOrLocal', () => {
  it('refuses every shared private vector', () => {
    expect(vectors.private.length).toBeGreaterThan(0);
    for (const ip of vectors.private) {
      expect(isPrivateOrLocal(ip), ip).toBe(true);
    }
  });

  it('allows every shared public vector', () => {
    expect(vectors.public.length).toBeGreaterThan(0);
    for (const ip of vectors.public) {
      expect(isPrivateOrLocal(ip), ip).toBe(false);
    }
  });

  it('refuses a value that is not an address', () => {
    expect(isPrivateOrLocal('example.com')).toBe(true);
    expect(isPrivateOrLocal('')).toBe(true);
  });
});

describe('checkedLookup', () => {
  it('returns every address when all are public', async () => {
    const resolve = vi.fn(async () => [{ address: '1.1.1.1', family: 4 }, { address: '2606:4700:4700::1111', family: 6 }]);
    await expect(checkedLookup('one.example', resolve)).resolves.toEqual([
      { address: '1.1.1.1', family: 4 },
      { address: '2606:4700:4700::1111', family: 6 },
    ]);
  });

  it('refuses the name when any address is private', async () => {
    const resolve = vi.fn(async () => [{ address: '1.1.1.1', family: 4 }, { address: '10.0.0.7', family: 4 }]);
    await expect(checkedLookup('mixed.example', resolve)).rejects.toBeInstanceOf(SsrfRefusedError);
  });

  it('refuses a name that resolves to nothing', async () => {
    await expect(checkedLookup('empty.example', async () => [])).rejects.toBeInstanceOf(SsrfRefusedError);
  });
});

describe('guardedLookup', () => {
  const two = async () => [{ address: '1.1.1.1', family: 4 }, { address: '8.8.8.8', family: 4 }];

  it('answers every address when asked for all', async () => {
    const got = await new Promise<unknown[]>((resolve) => {
      guardedLookup(two, vi.fn())('two.example', { all: true }, (_err, addrs) => resolve([addrs]));
    });
    expect(got).toEqual([[{ address: '1.1.1.1', family: 4 }, { address: '8.8.8.8', family: 4 }]]);
  });

  it('answers the first address otherwise', async () => {
    const got = await new Promise<unknown[]>((resolve) => {
      guardedLookup(two, vi.fn())('two.example', {}, (_err, addr, family) => resolve([addr, family]));
    });
    expect(got).toEqual(['1.1.1.1', 4]);
  });

  it('passes a resolver failure through without a refusal log', async () => {
    const log = vi.fn();
    const failing = async (): Promise<never> => {
      throw new Error('ENOTFOUND');
    };
    const err = await new Promise<Error | null>((resolve) => {
      guardedLookup(failing, log)('gone.example', {}, e => resolve(e));
    });
    expect(err?.message).toBe('ENOTFOUND');
    expect(log).not.toHaveBeenCalled();
  });
});

describe('rUNTIME_ALLOW_PRIVATE', () => {
  it('refuses an entry that is not host:port', () => {
    expect(() => installGuard({ env: { RUNTIME_ALLOW_PRIVATE: 'localhost' }, log: vi.fn() })).toThrow('is not host:port');
  });

  it('accepts a bracketed IPv6 entry', () => {
    const log = vi.fn();
    installGuard({ env: { RUNTIME_ENV: 'development', RUNTIME_ALLOW_PRIVATE: '[::1]:5190, Localhost:5190' }, log });
    expect(log).toHaveBeenCalledWith('private egress allowed', { hosts: ['::1:5190', 'localhost:5190'] });
  });

  it('logs JSON to stderr by default', () => {
    const spy = vi.spyOn(console, 'warn').mockImplementation(() => {});
    installGuard({ env: { NODE_USE_ENV_PROXY: '1' } });
    expect(spy).toHaveBeenCalledWith(JSON.stringify({ level: 'warn', msg: 'proxy env ignored', vars: ['NODE_USE_ENV_PROXY'] }));
    spy.mockRestore();
  });
});

describe('installGuard', () => {
  let target: { server: Server, port: number };
  let redirector: { server: Server, port: number };
  const hits: string[] = [];
  const logged: [string, Record<string, unknown>][] = [];
  const log = (msg: string, fields: Record<string, unknown>): void => {
    logged.push([msg, fields]);
  };

  beforeAll(async () => {
    target = await listen((req, res) => {
      hits.push(req.url ?? '');
      res.end('private');
    });
    redirector = await listen((_req, res) => {
      res.writeHead(302, { Location: `http://127.0.0.1:${target.port}/secret` });
      res.end();
    });
    installGuard({
      env: { RUNTIME_ENV: 'development', RUNTIME_ALLOW_PRIVATE: `127.0.0.1:${redirector.port}`, HTTP_PROXY: 'http://127.0.0.1:9', HTTPS_PROXY: 'http://127.0.0.1:9' },
      log,
    });
  });

  afterAll(() => {
    target.server.close();
    redirector.server.close();
  });

  it('makes the guarded agent the global dispatcher', () => {
    const agent = installGuard({ env: { RUNTIME_ENV: 'development', RUNTIME_ALLOW_PRIVATE: `127.0.0.1:${redirector.port}` }, log: vi.fn() });
    expect(getGlobalDispatcher()).toBe(agent);
  });

  it('refuses a private address literal', async () => {
    expect(await refusal(`http://127.0.0.1:${target.port}/`)).toBeInstanceOf(SsrfRefusedError);
    expect(await refusal(`http://[::1]:${target.port}/`)).toBeInstanceOf(SsrfRefusedError);
    expect(await refusal(`http://[::ffff:127.0.0.1]:${target.port}/`)).toBeInstanceOf(SsrfRefusedError);
    expect(await refusal('https://127.0.0.1/')).toBeInstanceOf(SsrfRefusedError);
  });

  it('does not refuse a public address literal', async () => {
    let cause: unknown;
    try {
      await fetch('http://1.1.1.1:81/', { signal: AbortSignal.timeout(300) });
    } catch (error) {
      cause = (error as Error).cause ?? error;
    }
    expect(cause).not.toBeInstanceOf(SsrfRefusedError);
    expect(logged).not.toContainEqual(['ssrf refused', { host: '1.1.1.1' }]);
  });

  it('refuses localhost at lookup', async () => {
    const cause = await refusal(`http://localhost:${target.port}/`);
    expect(cause).toBeInstanceOf(SsrfRefusedError);
    expect((cause as SsrfRefusedError).message).toBe('ssrf refused localhost');
  });

  it('lets an allow-listed host:port through', async () => {
    const res = await fetch(`http://127.0.0.1:${redirector.port}/`, { redirect: 'manual' });
    expect(res.status).toBe(302);
  });

  it('refuses a redirect hop to a private address', async () => {
    expect(await refusal(`http://127.0.0.1:${redirector.port}/`)).toBeInstanceOf(SsrfRefusedError);
    expect(hits).toEqual([]);
  });

  it('ignores the proxy env and says so at start', async () => {
    expect(logged).toContainEqual(['proxy env ignored', { vars: ['HTTP_PROXY', 'HTTPS_PROXY'] }]);
    expect(logged).toContainEqual(['private egress allowed', { hosts: [`127.0.0.1:${redirector.port}`] }]);
    const res = await fetch(`http://127.0.0.1:${redirector.port}/`, { redirect: 'manual' });
    expect(res.status).toBe(302);
  });

  it('ignores the allow-list in production, the default, and says so', async () => {
    const quietLog = vi.fn();
    installGuard({ env: { RUNTIME_ALLOW_PRIVATE: `127.0.0.1:${redirector.port}` }, log: quietLog });
    expect(quietLog).toHaveBeenCalledWith('private egress list ignored in production', { hosts: [`127.0.0.1:${redirector.port}`] });
    const cause = await fetch(`http://127.0.0.1:${redirector.port}/`, { redirect: 'manual' }).then(
      () => undefined,
      (error: Error) => error.cause,
    );
    expect(cause).toBeInstanceOf(SsrfRefusedError);
    expect(() => installGuard({ env: { RUNTIME_ENV: 'staging' }, log: vi.fn() })).toThrow('RUNTIME_ENV');
    installGuard({ env: { RUNTIME_ENV: 'development', RUNTIME_ALLOW_PRIVATE: `127.0.0.1:${redirector.port}` }, log: vi.fn() });
  });

  it('says nothing about proxies when none is set', () => {
    const quiet = vi.fn();
    installGuard({ env: {}, log: quiet });
    expect(quiet).not.toHaveBeenCalled();
    installGuard({ env: { RUNTIME_ENV: 'development', RUNTIME_ALLOW_PRIVATE: `127.0.0.1:${redirector.port}` }, log: vi.fn() });
  });
});
