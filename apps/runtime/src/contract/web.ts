export const SEARCH_KINDS = ['brave', 'exa'] as const;

export type SearchKind = typeof SEARCH_KINDS[number];

export const RESERVED_HEADERS: ReadonlySet<string> = new Set([
  'host',
  'content-length',
  'transfer-encoding',
  'connection',
  'keep-alive',
  'te',
  'trailer',
  'upgrade',
  'expect',
  'proxy-authorization',
  'proxy-connection',
  'cookie',
  'forwarded',
  'x-forwarded-for',
  'x-forwarded-host',
  'x-forwarded-proto',
  'x-real-ip',
  'x-http-method-override',
  'x-http-method',
  'x-method-override',
]);

export const MODEL_REFUSED_HEADERS: ReadonlySet<string> = new Set(['authorization']);

const LABEL = /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/;

export function validHost(host: string): boolean {
  const labels = host.split('.');
  return host.length <= 253
    && labels.length >= 2
    && labels.every(l => LABEL.test(l))
    && !/^(?:\d+|0x[\da-f]*)$/.test(labels.at(-1)!);
}
