import { isIP } from 'node:net';

import { isPrivateOrLocal } from './ssrf.ts';

const MAX_REDIRECTS = 5;
const MAX_ERROR_BODY = 512;
const MAX_ERROR_READ = 4096;
const REDIRECT_STATUS = new Set([301, 302, 303, 307, 308]);
const REDACTED = '[redacted]';

export interface Credential {
  header: string;
  value: string;
  secrets: string[];
}

export function origin(u: URL): string {
  const port = u.port || (u.protocol === 'https:' ? '443' : '80');
  return `${u.protocol}//${u.hostname.toLowerCase()}:${port}`;
}

export function redact(text: string, secrets: string[]): string {
  let out = text;
  for (const secret of [...secrets].sort((a, b) => b.length - a.length)) {
    if (secret) {
      out = out.split(secret).join(REDACTED);
    }
  }
  return out;
}

async function readCapped(res: Response): Promise<string> {
  const reader = res.body?.getReader();
  if (!reader) {
    return '';
  }
  const chunks: Uint8Array[] = [];
  let total = 0;
  while (total < MAX_ERROR_READ) {
    const { done, value } = await reader.read();
    if (done) {
      break;
    }
    chunks.push(value);
    total += value.length;
  }
  await reader.cancel().catch(() => {});
  const bytes = new Uint8Array(total);
  let at = 0;
  for (const c of chunks) {
    bytes.set(c, at);
    at += c.length;
  }
  return new TextDecoder().decode(bytes.subarray(0, MAX_ERROR_READ));
}

async function safeErrorBody(res: Response, secrets: string[]): Promise<Response> {
  const text = redact(await readCapped(res), secrets);
  const headers = new Headers(res.headers);
  headers.delete('content-length');
  headers.delete('content-encoding');
  return new Response([...text].slice(0, MAX_ERROR_BODY).join(''), { status: res.status, statusText: res.statusText, headers });
}

export function guardedFetch(homeUrl: string, cred: Credential | undefined, fetchImpl: typeof fetch = globalThis.fetch): typeof fetch {
  const home = origin(new URL(homeUrl));
  const secrets = cred?.secrets ?? [];
  return (async (input: string | URL | Request, init: RequestInit = {}) => {
    let url = new URL(input instanceof Request ? input.url : String(input));
    let method = init.method ?? 'GET';
    let body = init.body;
    for (let hop = 0; ; hop++) {
      const headers = new Headers(init.headers);
      headers.delete('authorization');
      if (cred) {
        headers.delete(cred.header);
      }
      if (origin(url) === home) {
        if (cred) {
          headers.set(cred.header, cred.value);
        }
      } else {
        headers.delete('mcp-session-id');
      }
      const res = await fetchImpl(url, { ...init, method, body, headers, redirect: 'manual' });
      const location = res.headers.get('location');
      if (!REDIRECT_STATUS.has(res.status) || location === null) {
        return res.ok ? res : safeErrorBody(res, secrets);
      }
      if (hop >= MAX_REDIRECTS) {
        throw new Error('egress: too many redirects');
      }
      const next = new URL(location, url);
      if (next.protocol !== 'https:') {
        throw new Error('egress: redirect to a non-https URL');
      }
      const host = next.hostname.replace(/^\[(.*)\]$/, '$1');
      if (isIP(host) !== 0 && isPrivateOrLocal(host)) {
        throw new Error('egress: redirect to a private address');
      }
      if (res.status === 303 || ((res.status === 301 || res.status === 302) && method === 'POST')) {
        method = 'GET';
        body = undefined;
      }
      await res.body?.cancel();
      url = next;
    }
  }) as typeof fetch;
}
