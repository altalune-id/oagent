import type { IncomingMessage, Server, ServerResponse } from 'node:http';
import type { AddressInfo } from 'node:net';
import type { McpServerSpec } from './mcp.ts';
import { Buffer } from 'node:buffer';
import { readFileSync } from 'node:fs';
import { createServer } from 'node:http';
import { fileURLToPath } from 'node:url';
import { Agent, configureLogging } from '@strands-agents/sdk';
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from 'vitest';

import { createLogger, strandsLogger } from './log.ts';
import { openMcpServers, originFetch, RESERVED_HEADERS, sanitizeDescription } from './mcp.ts';
import { ScriptedModel } from './test/scripted-model.ts';

interface Seen {
  method: string;
  auth: string | undefined;
  custom: string | undefined;
}

const dirty = `Records\u0007 an\u200B expense.\nSecond line.${'x'.repeat(2000)}`;
const bigSchema = { type: 'object', properties: Object.fromEntries(Array.from({ length: 800 }, (_, i) => [`field_${i}`, { type: 'string', description: 'padding padding' }])) };
const tools = [
  { name: 'record_expense', description: 'Record an expense.', inputSchema: { type: 'object', properties: { slow: { type: 'boolean' } } } },
  { name: 'list_wallets', description: 'List wallets.', inputSchema: { type: 'object' } },
  { name: 'bad.name', description: 'Bad name.', inputSchema: { type: 'object' } },
  { name: 'big', description: 'Big schema.', inputSchema: bigSchema },
  { name: 'dirty', description: dirty, inputSchema: { type: 'object' } },
  { name: 'hidden', description: 'Hidden schema text.', inputSchema: { type: 'object', title: 'T\u200Bitle', properties: { note: { type: 'string', description: 'say \u{E0041}\u{E0042} hi', items: [{ title: 'x\u202Ey' }] } } } },
];

let seen: Seen[] = [];

async function readJson(req: IncomingMessage): Promise<{ id?: number, method: string, params?: Record<string, unknown> }> {
  const chunks: Buffer[] = [];
  for await (const c of req) {
    chunks.push(c as Buffer);
  }
  return JSON.parse(Buffer.concat(chunks).toString('utf8')) as { id?: number, method: string, params?: Record<string, unknown> };
}

function send(res: ServerResponse, id: number | undefined, result: unknown): void {
  res.writeHead(200, { 'content-type': 'application/json' }).end(JSON.stringify({ jsonrpc: '2.0', id, result }));
}

async function handle(req: IncomingMessage, res: ServerResponse): Promise<void> {
  if (req.url === '/hang') {
    return;
  }
  if (req.method !== 'POST') {
    res.writeHead(405).end();
    return;
  }
  const msg = await readJson(req);
  if (req.url === '/echo-init' || (req.url === '/echo-call' && msg.method === 'tools/call')) {
    res.writeHead(500, { 'content-type': 'text/plain' }).end(`upstream said no to ${req.headers.authorization}`);
    return;
  }
  seen.push({ method: msg.method, auth: req.headers.authorization, custom: req.headers['x-api-key'] as string | undefined });
  switch (msg.method) {
    case 'initialize':
      send(res, msg.id, { protocolVersion: msg.params?.protocolVersion, capabilities: { tools: {} }, serverInfo: { name: 'fake', version: '1' } });
      return;
    case 'tools/list':
      send(res, msg.id, { tools });
      return;
    case 'tools/call': {
      const args = msg.params?.arguments as { slow?: boolean } | undefined;
      if (args?.slow) {
        setTimeout(send, 2000, res, msg.id, { content: [{ type: 'text', text: 'late' }] });
        return;
      }
      send(res, msg.id, { content: [{ type: 'text', text: 'ok' }] });
      return;
    }
    default:
      if (msg.id === undefined) {
        res.writeHead(202).end();
        return;
      }
      res.writeHead(200, { 'content-type': 'application/json' }).end(JSON.stringify({ jsonrpc: '2.0', id: msg.id, error: { code: -32601, message: 'not found' } }));
  }
}

let server: Server;
let base: string;
const quiet = createLogger(() => {});

function spec(over: Partial<McpServerSpec> = {}): McpServerSpec {
  return {
    name: 'yasaku',
    url: `${base}/mcp`,
    auth: { kind: 'bearer', value: 'tok-test' },
    allowedTools: ['record_expense', 'bad.name', 'big', 'dirty'],
    connectTimeoutMs: 2000,
    callTimeoutMs: 300,
    ...over,
  };
}

beforeAll(async () => {
  server = createServer((req, res) => {
    void handle(req, res);
  });
  await new Promise<void>(resolve => server.listen(0, '127.0.0.1', resolve));
  base = `http://127.0.0.1:${(server.address() as AddressInfo).port}`;
});

afterEach(() => {
  seen = [];
});

afterAll(() => {
  server.closeAllConnections();
  server.close();
});

describe('openMcpServers', () => {
  it('prefixes tool names, keeps only allowed ones and skips bad names and big schemas', async () => {
    const turn = await openMcpServers([spec()], quiet);
    expect(turn.tools.map(t => t.name).sort()).toEqual(['yasaku__dirty', 'yasaku__record_expense']);
    expect(turn.notes).toEqual([]);
    expect(turn.promptLines).toEqual([]);
    await turn.close();
  });

  it('sanitizes tool descriptions', async () => {
    const turn = await openMcpServers([spec()], quiet);
    const tool = turn.tools.find(t => t.name === 'yasaku__dirty')!;
    expect(tool.description).toBe(sanitizeDescription(dirty));
    expect(tool.toolSpec.description).toBe(tool.description);
    expect(tool.description.includes('\u0007')).toBe(false);
    expect(tool.description.includes('\u200B')).toBe(false);
    expect(tool.description).toContain('\nSecond line.');
    expect([...tool.description]).toHaveLength(1024);
    await turn.close();
  });

  it('negotiates the legacy protocol and never reconnects a turn client', async () => {
    const turn = await openMcpServers([spec()], quiet);
    const client = turn.servers[0]!.client;
    await client.connect();
    await expect(client.connect(true)).rejects.toThrow('never reconnects');
    await client.listTools();
    const methods = seen.map(s => s.method);
    expect(methods.filter(m => m === 'initialize')).toHaveLength(1);
    expect(methods).not.toContain('server/discover');
    expect(methods).not.toContain('subscriptions/listen');
    await turn.close();
  });

  it('sends the bearer credential to the server', async () => {
    const turn = await openMcpServers([spec()], quiet);
    expect(seen.length).toBeGreaterThan(0);
    expect(seen.every(s => s.auth === 'Bearer tok-test')).toBe(true);
    await turn.close();
  });

  it('sends a header credential under its own name', async () => {
    const turn = await openMcpServers([spec({ auth: { kind: 'header', header: 'X-Api-Key', value: 'key-test' } })], quiet);
    expect(seen.every(s => s.custom === 'key-test' && s.auth === undefined)).toBe(true);
    await turn.close();
  });

  it('applies the call timeout', async () => {
    const turn = await openMcpServers([spec()], quiet);
    const server0 = turn.servers[0]!;
    const tool = turn.tools.find(t => t.name === 'yasaku__record_expense')!;
    const started = Date.now();
    await expect(server0.client.callTool(tool as never, { slow: true })).rejects.toThrow();
    expect(Date.now() - started).toBeLessThan(1500);
    await turn.close();
  });

  it('drops a down server with a note and a prompt line', async () => {
    const lines: string[] = [];
    const turn = await openMcpServers([spec(), spec({ name: 'gone', url: 'http://127.0.0.1:1/mcp' })], createLogger(l => lines.push(l)));
    expect(turn.tools.map(t => t.name).sort()).toEqual(['yasaku__dirty', 'yasaku__record_expense']);
    expect(turn.notes).toEqual([{ kind: 'mcp_server_unavailable', server: 'gone' }]);
    expect(turn.promptLines).toEqual(['Tools from gone are unavailable in this turn.']);
    expect(lines.join('\n')).toContain('"server":"gone"');
    await turn.close();
  });

  it('gives up on a server that does not answer within the connect timeout', async () => {
    const started = Date.now();
    const turn = await openMcpServers([spec({ name: 'slow', url: `${base}/hang`, connectTimeoutMs: 200 })], quiet);
    expect(Date.now() - started).toBeLessThan(1500);
    expect(turn.notes).toEqual([{ kind: 'mcp_server_unavailable', server: 'slow' }]);
    await turn.close();
  });

  it('refuses a credential header with CR, LF or control characters before any request', async () => {
    const bad = [
      spec({ name: 'crlf', auth: { kind: 'bearer', value: 'tok\r\nX-Evil: 1' } }),
      spec({ name: 'ctl', auth: { kind: 'header', header: 'X-Api-Key', value: 'a\u0000b' } }),
      spec({ name: 'hname', auth: { kind: 'header', header: 'X Api', value: 'v' } }),
      spec({ name: 'nohdr', auth: { kind: 'header', value: 'v' } }),
      spec({ name: 'noval', auth: { kind: 'bearer' } }),
    ];
    const turn = await openMcpServers(bad, quiet);
    expect(turn.tools).toEqual([]);
    expect(turn.notes.map(n => n.server)).toEqual(['crlf', 'ctl', 'hname', 'nohdr', 'noval']);
    expect(seen).toEqual([]);
    await turn.close();
  });

  it('connects without a credential when the auth kind is none', async () => {
    const turn = await openMcpServers([spec({ auth: { kind: 'none' } })], quiet);
    expect(turn.tools).toHaveLength(2);
    expect(seen.every(s => s.auth === undefined)).toBe(true);
    await turn.close();
  });
});

describe('remote text and secrets', () => {
  it('sanitizes titles and descriptions at every depth of the input schema', async () => {
    const turn = await openMcpServers([spec({ allowedTools: ['hidden'] })], quiet);
    const schema = JSON.stringify(turn.tools[0]!.toolSpec.inputSchema);
    expect(schema).not.toMatch(/[\u200B\u202E]/);
    expect(schema).not.toContain('\u{E0041}');
    expect(schema).toContain('"description":"say  hi"');
    expect(schema).toContain('"title":"Title"');
    await turn.close();
  });

  it('reserves exactly the header names Go reserves', () => {
    const shared = JSON.parse(readFileSync(fileURLToPath(new URL('../../../internal/mcpserver/testdata/reserved_headers.json', import.meta.url)), 'utf8')) as string[];
    expect([...RESERVED_HEADERS].sort()).toEqual(shared.map(h => h.toLowerCase()).sort());
  });

  it('refuses a reserved header name, as Go does at save', async () => {
    const turn = await openMcpServers([
      spec({ name: 'h1', auth: { kind: 'header', header: 'Host', value: 'v' } }),
      spec({ name: 'h2', auth: { kind: 'header', header: 'MCP-Session-Id', value: 'v' } }),
      spec({ name: 'h3', auth: { kind: 'header', header: 'Proxy-Authorization', value: 'v' } }),
    ], quiet);
    expect(turn.notes.map(n => n.server)).toEqual(['h1', 'h2', 'h3']);
    expect(seen).toEqual([]);
    await turn.close();
  });

  it('keeps an echoed bearer out of every log line and out of the tool result', async () => {
    const lines: string[] = [];
    const log = createLogger(l => lines.push(l));
    configureLogging(strandsLogger(log));
    const consoleLines: string[] = [];
    const spies = (['log', 'info', 'warn', 'error', 'debug'] as const).map(k => vi.spyOn(console, k).mockImplementation((...a: unknown[]) => {
      consoleLines.push(a.map(String).join(' '));
    }));
    try {
      const down = await openMcpServers([spec({ name: 'echoinit', url: `${base}/echo-init` })], log);
      expect(down.notes).toEqual([{ kind: 'mcp_server_unavailable', server: 'echoinit' }]);
      const turn = await openMcpServers([spec({ url: `${base}/echo-call`, allowedTools: ['record_expense'] })], log);
      const agent = new Agent({
        model: new ScriptedModel([{ toolUse: { name: 'yasaku__record_expense', id: 'c1', input: {} } }, { text: ['done'] }]),
        tools: turn.tools,
        sandbox: false,
        printer: false,
      });
      await agent.invoke('go');
      const result = JSON.stringify(agent.messages);
      expect(result).toContain('[redacted]');
      expect(result).not.toContain('tok-test');
      await turn.close();
    } finally {
      spies.forEach(s => s.mockRestore());
    }
    expect([...lines, ...consoleLines].join('\n')).not.toContain('tok-test');
  });
});

describe('originFetch', () => {
  interface Call { url: string; auth: string | null; method: string; body: unknown; }

  function fakeFetch(routes: Record<string, { status: number, location?: string }>): { fetch: typeof fetch, calls: Call[] } {
    const calls: Call[] = [];
    const impl = (async (input: string | URL, init?: RequestInit) => {
      const url = String(input);
      const headers = new Headers(init?.headers);
      calls.push({ url, auth: headers.get('authorization'), method: init?.method ?? 'GET', body: init?.body });
      const route = routes[url] ?? { status: 200 };
      return new Response(null, { status: route.status, headers: route.location ? { location: route.location } : {} });
    }) as typeof fetch;
    return { fetch: impl, calls };
  }

  const auth = { kind: 'bearer' as const, value: 'tok-test' };

  it('keeps the credential on a same-origin redirect', async () => {
    const f = fakeFetch({ 'https://mcp.example.com/mcp': { status: 307, location: '/v2/mcp' } });
    const res = await originFetch('https://mcp.example.com/mcp', auth, f.fetch)('https://mcp.example.com/mcp', { method: 'POST', body: '{}' });
    expect(res.status).toBe(200);
    expect(f.calls).toEqual([
      { url: 'https://mcp.example.com/mcp', auth: 'Bearer tok-test', method: 'POST', body: '{}' },
      { url: 'https://mcp.example.com/v2/mcp', auth: 'Bearer tok-test', method: 'POST', body: '{}' },
    ]);
  });

  it('drops the credential on a cross-origin redirect, also one set by the caller', async () => {
    const f = fakeFetch({ 'https://mcp.example.com/mcp': { status: 302, location: 'https://evil.example.net/x' } });
    await originFetch('https://mcp.example.com/mcp', auth, f.fetch)('https://mcp.example.com/mcp', { method: 'POST', body: '{}', headers: { authorization: 'Bearer caller' } });
    expect(f.calls[1]).toEqual({ url: 'https://evil.example.net/x', auth: null, method: 'GET', body: undefined });
  });

  it('treats a different port as another origin', async () => {
    const f = fakeFetch({ 'https://mcp.example.com/mcp': { status: 308, location: 'https://mcp.example.com:8443/mcp' } });
    await originFetch('https://mcp.example.com/mcp', auth, f.fetch)('https://mcp.example.com/mcp', { method: 'POST', body: '{}' });
    expect(f.calls[1]!.auth).toBeNull();
  });

  it('refuses a redirect to http, to a private literal, and past five hops', async () => {
    const http = fakeFetch({ 'https://mcp.example.com/mcp': { status: 302, location: 'http://mcp.example.com/mcp' } });
    await expect(originFetch('https://mcp.example.com/mcp', auth, http.fetch)('https://mcp.example.com/mcp')).rejects.toThrow('redirect to a non-https URL');
    const priv = fakeFetch({ 'https://mcp.example.com/mcp': { status: 302, location: 'https://169.254.169.254/latest' } });
    await expect(originFetch('https://mcp.example.com/mcp', auth, priv.fetch)('https://mcp.example.com/mcp')).rejects.toThrow('redirect to a private address');
    const loop = fakeFetch({ 'https://mcp.example.com/mcp': { status: 302, location: 'https://mcp.example.com/mcp' } });
    await expect(originFetch('https://mcp.example.com/mcp', auth, loop.fetch)('https://mcp.example.com/mcp')).rejects.toThrow('too many redirects');
    expect(loop.calls).toHaveLength(6);
  });

  it('returns a redirect without a location as it is', async () => {
    const f = fakeFetch({ 'https://mcp.example.com/mcp': { status: 302 } });
    const res = await originFetch('https://mcp.example.com/mcp', auth, f.fetch)('https://mcp.example.com/mcp');
    expect(res.status).toBe(302);
  });
});
