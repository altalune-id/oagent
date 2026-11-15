import type { WebTurn } from '../turn/web/call.ts';
import { vi } from 'vitest';

import { emptyUsage } from '../turn/limits.ts';
import { WebCalls } from '../turn/web/call.ts';
import { captureLogger } from './logger.ts';

export type Sent = { url: string, method: string, headers: Headers, body: string | undefined };

export function stubFetch(respond: (url: string, init: RequestInit) => Response | Promise<Response>): Sent[] {
  const sent: Sent[] = [];
  vi.stubGlobal('fetch', async (input: string | URL, init: RequestInit = {}) => {
    sent.push({ url: String(input), method: init.method ?? 'GET', headers: new Headers(init.headers), body: typeof init.body === 'string' ? init.body : undefined });
    return respond(String(input), init);
  });
  return sent;
}

export function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { 'content-type': 'application/json' } });
}

export async function failed<E extends Error>(pending: Promise<unknown>): Promise<E> {
  return pending.then(() => {
    throw new Error('expected a failure');
  }, (e: unknown) => e as E);
}

export function webTurn(over: Partial<WebTurn> = {}): WebTurn & { lines: string[] } {
  const lines: string[] = [];
  return { run: 'r1', log: captureLogger(lines), calls: new WebCalls(10), secrets: [], timeoutMs: 15_000, usage: emptyUsage(), budget: null, lines, ...over };
}
