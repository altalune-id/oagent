import { isIP } from 'node:net';

import { isPrivateOrLocal } from './ssrf.ts';

const MAX_REDIRECTS = 5;
const MAX_ERROR_BODY = 512;
const MAX_ERROR_READ = 4096;
const REDIRECT_STATUS = new Set([301, 302, 303, 307, 308]);
const REDACTED = '[redacted]';

export type Credential = {
  header: string,
  value: string,
  secrets: string[],
};

export type HostCredential = {
  header: string,
  value: string,
};

export type EgressOptions = {
  checkHop?: (url: URL) => void,
  followRedirects?: boolean,
  hostCredentials?: ReadonlyMap<string, readonly HostCredential[]>,
};

export type CappedBody = {
  bytes: Uint8Array,
  truncated: boolean,
};

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

export function secretForms(values: readonly string[]): string[] {
  return values.flatMap((v) => {
    const token = /^\S+\s+(\S.*)$/.exec(v)?.[1];
    return token === undefined ? [v] : [v, token];
  });
}

export async function readCapped(res: Response, maxBytes: number): Promise<CappedBody> {
  const reader = res.body?.getReader();
  if (!reader) {
    return { bytes: new Uint8Array(0), truncated: false };
  }
  const chunks: Uint8Array[] = [];
  let total = 0;
  while (total <= maxBytes) {
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
  return { bytes: bytes.subarray(0, maxBytes), truncated: total > maxBytes };
}

async function safeErrorBody(res: Response, secrets: string[]): Promise<Response> {
  const text = redact(new TextDecoder().decode((await readCapped(res, MAX_ERROR_READ)).bytes), secrets);
  const headers = new Headers(res.headers);
  headers.delete('content-length');
  headers.delete('content-encoding');
  return new Response([...text].slice(0, MAX_ERROR_BODY).join(''), { status: res.status, statusText: res.statusText, headers });
}

export function guardedFetch(homeUrl: string, cred: Credential | undefined, fetchImpl: typeof fetch = globalThis.fetch, options: EgressOptions = {}): typeof fetch {
  const home = origin(new URL(homeUrl));
  const hostCredentials = options.hostCredentials ?? new Map<string, readonly HostCredential[]>();
  const secrets = [...(cred?.secrets ?? []), ...secretForms([...hostCredentials.values()].flat().map(c => c.value))];
  return (async (input: string | URL | Request, init: RequestInit = {}) => {
    let url = new URL(input instanceof Request ? input.url : String(input));
    let method = init.method ?? 'GET';
    let body = init.body;
    for (let hop = 0; ; hop++) {
      options.checkHop?.(url);
      const headers = new Headers(init.headers);
      headers.delete('authorization');
      for (const c of hostCredentials.get(url.hostname) ?? []) {
        headers.set(c.header, c.value);
      }
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
      if (!REDIRECT_STATUS.has(res.status) || location === null || options.followRedirects === false) {
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
