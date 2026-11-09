import type { TurnRequest } from '../contract/index.ts';
import type { Credential } from '../egress/index.ts';

const HEADER_NAME = /^[!#$%&'*+.^`|~\w-]+$/;
export const RESERVED_HEADERS = new Set([
  'host',
  'content-length',
  'transfer-encoding',
  'connection',
  'content-type',
  'accept',
  'mcp-session-id',
  'mcp-protocol-version',
  'te',
  'trailer',
  'upgrade',
  'keep-alive',
  'proxy-authorization',
  'proxy-connection',
  'expect',
]);

export type McpAuth = TurnRequest['mcpServers'][number]['auth'];

export function credential(auth: McpAuth): Credential | undefined {
  if (auth.kind === 'bearer') {
    return { header: 'authorization', value: `Bearer ${auth.value}`, secrets: [`Bearer ${auth.value}`, auth.value!] };
  }
  if (auth.kind === 'header') {
    return { header: auth.header!, value: auth.value!, secrets: [auth.value!] };
  }
  return undefined;
}

function hasControl(value: string): boolean {
  return [...value].some((c) => {
    const code = c.codePointAt(0)!;
    return code < 0x20 || code === 0x7F;
  });
}

export function authProblem(auth: McpAuth): string | undefined {
  if (auth.kind === 'none') {
    return undefined;
  }
  if (!auth.value || hasControl(auth.value)) {
    return 'bad_header_value';
  }
  if (auth.kind === 'header' && (!auth.header || !HEADER_NAME.test(auth.header))) {
    return 'bad_header_name';
  }
  if (auth.kind === 'header' && RESERVED_HEADERS.has(auth.header!.toLowerCase())) {
    return 'reserved_header_name';
  }
  return undefined;
}
