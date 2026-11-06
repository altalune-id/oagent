import type { McpCallToolOptions, McpToolFilterCallback } from '@strands-agents/sdk';
import type { Credential } from './egress.ts';
import type { Logger } from './log.ts';
import { StreamableHTTPClientTransport } from '@modelcontextprotocol/client';

import { McpClient } from '@strands-agents/sdk';
import { guardedFetch } from './egress.ts';

const MAX_DESCRIPTION_RUNES = 1024;
const MAX_SCHEMA_BYTES = 16 * 1024;
const TOOL_NAME = /^[\w-]{1,64}$/;
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

export type McpTool = Parameters<McpToolFilterCallback>[0];

export interface McpAuth {
  kind: 'none' | 'bearer' | 'header';
  header?: string;
  value?: string;
}

export interface McpServerSpec {
  name: string;
  url: string;
  auth: McpAuth;
  allowedTools: string[];
  connectTimeoutMs: number;
  callTimeoutMs: number;
}

export interface Note {
  kind: 'mcp_server_unavailable';
  server: string;
}

export interface OpenServer {
  name: string;
  client: McpClient;
}

export interface McpTurn {
  servers: OpenServer[];
  tools: McpTool[];
  notes: Note[];
  promptLines: string[];
  close: () => Promise<void>;
}

export function sanitizeDescription(text: string): string {
  const cleaned = text.replace(/[\p{Cc}\p{Cf}\p{Co}]/gu, c => (c === '\n' ? c : ''));
  return [...cleaned].slice(0, MAX_DESCRIPTION_RUNES).join('');
}

function credential(auth: McpAuth): Credential | undefined {
  if (auth.kind === 'bearer') {
    return { header: 'authorization', value: `Bearer ${auth.value}`, secrets: [`Bearer ${auth.value}`, auth.value!] };
  }
  if (auth.kind === 'header') {
    return { header: auth.header!, value: auth.value!, secrets: [auth.value!] };
  }
  return undefined;
}

export function sanitizeSchemaValue(value: unknown): unknown {
  if (Array.isArray(value)) {
    return value.map(sanitizeSchemaValue);
  }
  if (value && typeof value === 'object') {
    return Object.fromEntries(Object.entries(value).map(([k, v]) => [
      k,
      typeof v === 'string' && (k === 'description' || k === 'title') ? sanitizeDescription(v) : sanitizeSchemaValue(v),
    ]));
  }
  return value;
}

function hasControl(value: string): boolean {
  return [...value].some((c) => {
    const code = c.codePointAt(0)!;
    return code < 0x20 || code === 0x7F;
  });
}

function authProblem(auth: McpAuth): string | undefined {
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

export function originFetch(serverUrl: string, auth: McpAuth, fetchImpl: typeof fetch = globalThis.fetch): typeof fetch {
  return guardedFetch(serverUrl, credential(auth), fetchImpl);
}

class TurnMcpClient extends McpClient {
  private readonly callTimeoutMs: number;

  constructor(spec: McpServerSpec, log: Logger) {
    super({
      transport: new StreamableHTTPClientTransport(new URL(spec.url), { fetch: originFetch(spec.url, spec.auth) }),
      applicationName: spec.name,
      logHandler: () => log.info('mcp server log', { server: spec.name }),
      prefix: `${spec.name}_`,
      continueOnError: true,
      toolFilters: {
        allowed: spec.allowedTools,
        rejected: [(tool: McpTool) => !TOOL_NAME.test(tool.name)],
      },
    });
    this.callTimeoutMs = spec.callTimeoutMs;
  }

  override async connect(reconnect?: boolean, options?: { signal?: AbortSignal }): Promise<void> {
    if (reconnect) {
      throw new Error('mcp: a turn client never reconnects; open a new one');
    }
    if (!this.client.transport) {
      this.client.setVersionNegotiation({ mode: 'legacy' });
    }
    await super.connect(false, options);
  }

  override callTool(tool: McpTool, args: Parameters<McpClient['callTool']>[1], options?: McpCallToolOptions): ReturnType<McpClient['callTool']> {
    return super.callTool(tool, args, { ...options, timeoutMs: this.callTimeoutMs });
  }
}

function withTimeout<T>(work: Promise<T>, ms: number): Promise<T> {
  return Promise.race([
    work,
    new Promise<never>((_, reject) => {
      setTimeout(() => reject(new Error('mcp: list timed out')), ms).unref();
    }),
  ]);
}

async function openOne(spec: McpServerSpec, log: Logger): Promise<{ client: McpClient, tools: McpTool[] } | string> {
  const problem = authProblem(spec.auth);
  if (problem) {
    return problem;
  }
  const client = new TurnMcpClient(spec, log);
  try {
    await client.connect(false, { signal: AbortSignal.timeout(spec.connectTimeoutMs) });
    if (client.connectionState !== 'connected') {
      await client.disconnect();
      return 'connect_failed';
    }
    const listed = await withTimeout(client.listTools(), spec.connectTimeoutMs);
    const tools: McpTool[] = [];
    for (const tool of listed) {
      const description = sanitizeDescription(tool.description);
      const inputSchema = sanitizeSchemaValue(tool.toolSpec.inputSchema ?? { type: 'object' });
      if (JSON.stringify(inputSchema).length > MAX_SCHEMA_BYTES) {
        continue;
      }
      Object.assign(tool, { description });
      Object.assign(tool.toolSpec, { description, inputSchema });
      tools.push(tool);
    }
    return { client, tools };
  } catch (error) {
    log.warn('mcp list failed', { server: spec.name, error });
    await client.disconnect().catch(() => {});
    return 'list_failed';
  }
}

export async function openMcpServers(specs: McpServerSpec[], log: Logger): Promise<McpTurn> {
  const results = await Promise.all(specs.map(spec => openOne(spec, log)));
  const turn: McpTurn = { servers: [], tools: [], notes: [], promptLines: [], close: async () => {} };
  results.forEach((result, i) => {
    const name = specs[i]!.name;
    if (typeof result === 'string') {
      log.warn('mcp server unavailable', { server: name, code: result });
      turn.notes.push({ kind: 'mcp_server_unavailable', server: name });
      turn.promptLines.push(`Tools from ${name} are unavailable in this turn.`);
      return;
    }
    turn.servers.push({ name, client: result.client });
    turn.tools.push(...result.tools);
  });
  turn.close = async () => {
    await Promise.all(turn.servers.map(s => s.client.disconnect().catch(() => {})));
  };
  return turn;
}
