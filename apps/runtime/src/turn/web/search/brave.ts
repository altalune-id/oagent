import type { ErrorClass } from '../errors.ts';
import type { Raw } from './common.ts';
import type { SearchDriver } from './index.ts';
import { guardedFetch } from '../../../egress/index.ts';
import { WebToolError } from '../errors.ts';
import { hopCheck } from '../policy.ts';
import { stripTags } from '../text.ts';
import { billedJson, errorCode, MAX_RESULTS, normalize, statusClass } from './common.ts';

const ENDPOINT = 'https://api.search.brave.com/res/v1/web/search';
const HOST = 'api.search.brave.com';
const ENTITY: Record<string, string> = { '&lt;': '<', '&gt;': '>', '&quot;': '"', '&#39;': '\'', '&amp;': '&' };

type Result = { title?: unknown, url?: unknown, description?: unknown, page_age?: unknown };

function plain(html: unknown): unknown {
  return typeof html === 'string' ? stripTags(html).replace(/&(?:lt|gt|quot|#39|amp);/g, e => ENTITY[e]!) : html;
}

function failure(status: number, body: unknown): ErrorClass {
  const code = (body as { error?: { code?: unknown } } | undefined)?.error?.code;
  if (code === 'QUOTA_LIMITED') {
    return 'quota';
  }
  if (status === 401 || status === 403 || (status === 422 && code === 'SUBSCRIPTION_TOKEN_INVALID')) {
    return 'bad_key';
  }
  if (status === 429) {
    return 'rate_limited';
  }
  return statusClass(status);
}

export const brave: SearchDriver = {
  async search(query, apiKey, signal) {
    const url = new URL(ENDPOINT);
    url.searchParams.set('q', query);
    url.searchParams.set('count', String(MAX_RESULTS));
    const send = guardedFetch(ENDPOINT, { header: 'x-subscription-token', value: apiKey, secrets: [apiKey] }, globalThis.fetch, { checkHop: hopCheck([HOST]), followRedirects: false });
    const res = await send(url, { headers: { accept: 'application/json' }, signal });
    if (!res.ok) {
      throw new WebToolError(failure(res.status, await errorCode(res)), HOST, res.status);
    }
    const body = await billedJson(res, HOST) as { web?: { results?: Result[] } } | null;
    const results = body?.web?.results;
    if (results !== undefined && !Array.isArray(results)) {
      throw new WebToolError('upstream_error', HOST, res.status, { billed: true });
    }
    return normalize((results ?? []).map((r): Raw => ({ title: plain(r.title), url: r.url, snippet: plain(r.description), publishedAt: r.page_age })), [apiKey]);
  },
};
