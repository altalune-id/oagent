import { MODEL_REFUSED_HEADERS, RESERVED_HEADERS, validHost } from '../../contract/index.ts';
import { WebToolError } from './errors.ts';

export const RESPONSE_HEADERS: readonly string[] = ['content-type', 'content-length', 'location', 'retry-after', 'etag', 'last-modified'];

export function modelHeaders(headers: Record<string, string>, allowed: ReadonlySet<string>): Record<string, string> {
  return Object.fromEntries(Object.entries(headers).filter(([name]) => {
    const lower = name.toLowerCase();
    return allowed.has(lower) && !RESERVED_HEADERS.has(lower) && !MODEL_REFUSED_HEADERS.has(lower);
  }));
}

export function hopCheck(allow: readonly string[]): (url: URL) => void {
  return (url) => {
    const host = url.hostname;
    const ok = url.protocol === 'https:' && url.port === '' && (allow.length === 0 ? validHost(host) : allow.includes(host));
    if (!ok) {
      throw new WebToolError('blocked_host', host);
    }
  };
}
