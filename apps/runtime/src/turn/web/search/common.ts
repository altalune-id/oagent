import type { ErrorClass } from '../errors.ts';
import type { SearchResult } from './index.ts';
import { readCapped, redact } from '../../../egress/index.ts';
import { WebToolError } from '../errors.ts';

export const MAX_RESULTS = 5;
const MAX_CHARS = 300;
const MAX_BODY = 1024 * 1024;

export type Raw = {
  title?: unknown,
  url?: unknown,
  snippet?: unknown,
  publishedAt?: unknown,
};

function text(value: unknown, secrets: string[]): string {
  return typeof value === 'string' ? redact(value, secrets).replace(/\s+/g, ' ').trim().slice(0, MAX_CHARS) : '';
}

function isoDate(value: unknown): string | undefined {
  if (typeof value !== 'string') {
    return undefined;
  }
  const at = Date.parse(/(?:Z|[+-]\d\d:?\d\d)$/.test(value) ? value : `${value}Z`);
  return Number.isNaN(at) ? undefined : new Date(at).toISOString();
}

function webUrl(value: unknown): string | undefined {
  if (typeof value !== 'string' || !URL.canParse(value)) {
    return undefined;
  }
  const url = new URL(value);
  return url.protocol === 'https:' || url.protocol === 'http:' ? url.href : undefined;
}

export function normalize(raws: Raw[], secrets: string[]): SearchResult[] {
  const out: SearchResult[] = [];
  for (const raw of raws) {
    const url = webUrl(raw.url);
    if (url === undefined) {
      continue;
    }
    const publishedAt = isoDate(raw.publishedAt);
    out.push({ title: text(raw.title, secrets), url, snippet: text(raw.snippet, secrets), ...(publishedAt && { publishedAt }) });
    if (out.length === MAX_RESULTS) {
      break;
    }
  }
  return out;
}

export async function errorCode(res: Response): Promise<unknown> {
  try {
    return JSON.parse(await res.text()) as unknown;
  } catch {
    return undefined;
  }
}

export async function billedJson(res: Response, host: string): Promise<unknown> {
  try {
    const body = await readCapped(res, MAX_BODY);
    return JSON.parse(new TextDecoder().decode(body.bytes)) as unknown;
  } catch {
    throw new WebToolError('upstream_error', host, res.status, { billed: true });
  }
}

export function statusClass(status: number): ErrorClass {
  return status >= 400 && status < 500 ? 'invalid_request' : 'upstream_error';
}
