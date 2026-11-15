import type { WebTurn } from './call.ts';
import { tool } from '@strands-agents/sdk';
import { z } from 'zod';

import { guardedFetch, readCapped, redact } from '../../egress/index.ts';
import { runCall } from './call.ts';
import { WebToolError } from './errors.ts';
import { hopCheck, modelHeaders, RESPONSE_HEADERS } from './policy.ts';

const MAX_BODY_BYTES = 64 * 1024;
const MAX_READ = 1024 * 1024;
const MAX_TEXT_CHARS = 20_000;
const ERROR_BODY_CHARS = 512;
const MAX_HEADERS = 20;

export type HttpSpec = {
  hosts: string[],
  requestHeaders: string[],
  secrets: { host: string, header: string, value: string }[],
};

function texty(contentType: string): boolean {
  const type = contentType.split(';')[0]!.trim().toLowerCase();
  return type === '' || type.startsWith('text/') || /^application\/(?:json|xml|x-www-form-urlencoded|javascript|[\w.-]+\+(?:json|xml))$/.test(type);
}

function passes(check: (url: URL) => void, url: URL): boolean {
  try {
    check(url);
    return true;
  } catch {
    return false;
  }
}

export function httpTool(turn: WebTurn, spec: HttpSpec) {
  const check = hopCheck(spec.hosts);
  const allowed = new Set(spec.requestHeaders.map(h => h.toLowerCase()));
  const hostCredentials = new Map<string, { header: string, value: string }[]>();
  for (const s of spec.secrets) {
    hostCredentials.set(s.host, [...(hostCredentials.get(s.host) ?? []), { header: s.header, value: s.value }]);
  }
  return tool({
    name: 'http_request',
    description: `Call an HTTP API on one of these hosts: ${spec.hosts.join(', ')}. GET runs at once; POST, PUT, PATCH and DELETE wait for a person to approve. Returns {status, headers, body, truncated?}.`,
    inputSchema: z.strictObject({
      method: z.enum(['GET', 'POST', 'PUT', 'PATCH', 'DELETE']),
      url: z.string().url().max(2048).describe('The https URL'),
      headers: z.record(z.string(), z.string().max(4096)).refine(h => Object.keys(h).length <= MAX_HEADERS).optional().describe(`Request headers; only these are sent: ${spec.requestHeaders.join(', ') || 'none'}`),
      body: z.string().optional().describe('Request body, at most 64 KiB; not allowed on GET'),
      timeout_ms: z.number().int().min(1000).max(30_000).optional(),
    }),
    callback: (input, ctx) => runCall(turn, 'http_request', ctx, Math.min(input.timeout_ms ?? turn.timeoutMs, turn.timeoutMs), async (signal) => {
      const url = new URL(input.url);
      if (input.body !== undefined && (input.method === 'GET' || new TextEncoder().encode(input.body).length > MAX_BODY_BYTES)) {
        throw new WebToolError('invalid_request', url.hostname);
      }
      const headers = modelHeaders(input.headers ?? {}, allowed);
      const send = guardedFetch(input.url, undefined, globalThis.fetch, { checkHop: check, followRedirects: input.method === 'GET', hostCredentials });
      const res = await send(url, { method: input.method, headers, body: input.body, signal });
      const secrets = [...turn.secrets];
      const out: Record<string, string> = {};
      for (const name of RESPONSE_HEADERS) {
        const value = res.headers.get(name);
        if (value === null) {
          continue;
        }
        if (name === 'location') {
          const target = URL.canParse(value, url) ? new URL(value, url) : undefined;
          if (target && passes(check, target)) {
            out[name] = redact(target.href, secrets);
          }
          continue;
        }
        out[name] = redact(value, secrets);
      }
      let body: string;
      let truncated: boolean;
      let bytes: number;
      if (res.ok) {
        const read = await readCapped(res, MAX_READ);
        bytes = read.bytes.length;
        const text = redact(new TextDecoder().decode(read.bytes), secrets);
        body = texty(res.headers.get('content-type') ?? '') ? text.slice(0, MAX_TEXT_CHARS) : `[binary body: ${bytes} bytes]`;
        truncated = read.truncated || (texty(res.headers.get('content-type') ?? '') && text.length > MAX_TEXT_CHARS);
      } else {
        body = redact(await res.text(), secrets);
        bytes = body.length;
        truncated = [...body].length >= ERROR_BODY_CHARS;
      }
      return { text: JSON.stringify({ status: res.status, headers: out, body, ...(truncated && { truncated: true }) }), host: url.hostname, status: res.status, bytes };
    }),
  });
}
