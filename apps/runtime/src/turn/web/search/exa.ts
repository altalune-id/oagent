import type { ErrorClass } from '../errors.ts';
import type { Raw } from './common.ts';
import type { SearchDriver } from './index.ts';
import { guardedFetch } from '../../../egress/index.ts';
import { WebToolError } from '../errors.ts';
import { hopCheck } from '../policy.ts';
import { billedJson, MAX_RESULTS, normalize, statusClass } from './common.ts';

const ENDPOINT = 'https://api.exa.ai/search';
const HOST = 'api.exa.ai';

type Result = { title?: unknown, url?: unknown, publishedDate?: unknown, highlights?: unknown, summary?: unknown, text?: unknown };

function failure(status: number): ErrorClass {
  if (status === 401) {
    return 'bad_key';
  }
  if (status === 402) {
    return 'quota';
  }
  if (status === 429) {
    return 'rate_limited';
  }
  return statusClass(status);
}

function snippet(r: Result): unknown {
  const first = Array.isArray(r.highlights) ? r.highlights[0] as unknown : undefined;
  return [first, r.summary, r.text].find(v => typeof v === 'string' && v.trim() !== '');
}

export const exa: SearchDriver = {
  async search(query, apiKey, signal) {
    const send = guardedFetch(ENDPOINT, { header: 'x-api-key', value: apiKey, secrets: [apiKey] }, globalThis.fetch, { checkHop: hopCheck([HOST]), followRedirects: false });
    const res = await send(ENDPOINT, {
      method: 'POST',
      headers: { 'content-type': 'application/json', 'accept': 'application/json' },
      body: JSON.stringify({ query, type: 'auto', numResults: MAX_RESULTS, contents: { highlights: true } }),
      signal,
    });
    if (!res.ok) {
      await res.body?.cancel();
      throw new WebToolError(failure(res.status), HOST, res.status);
    }
    const body = await billedJson(res, HOST) as { results?: Result[] } | null;
    if (!Array.isArray(body?.results)) {
      throw new WebToolError('upstream_error', HOST, res.status, { billed: true });
    }
    return normalize(body.results.map((r): Raw => ({ title: r.title, url: r.url, snippet: snippet(r), publishedAt: r.publishedDate })), [apiKey]);
  },
};
