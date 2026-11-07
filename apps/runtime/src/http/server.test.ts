import type { Server } from 'node:http';
import type { AddressInfo } from 'node:net';
import type { RunTurn } from './server.ts';
import { readFileSync } from 'node:fs';
import { createServer, request as httpRequest } from 'node:http';
import { fileURLToPath } from 'node:url';

import { afterEach, describe, expect, it } from 'vitest';
import { createLogger } from '../log.ts';
import { ModelClients } from '../models/clients.ts';
import { ScriptedModel } from '../test/scripted-model.ts';
import { fakeMcp } from '../test/turn-fixtures.ts';
import { createRunTurn } from '../turn/agui.ts';
import { TurnRefusedError } from './contract.ts';
import { createRuntimeServer, loadConfig, MAX_BODY_BYTES, start } from './server.ts';

const TOKEN = 'a'.repeat(32);
const goldenBody = readFileSync(fileURLToPath(new URL('../../../../internal/kernel/runtime/testdata/turn/plain_reply.request.json', import.meta.url)), 'utf8');
const quiet = createLogger(() => {});
const servers: Server[] = [];

async function serve(runTurn: RunTurn, maxConcurrent = 8): Promise<string> {
  const server = createRuntimeServer({ token: TOKEN, maxConcurrent, log: quiet, runTurn });
  servers.push(server);
  await new Promise<void>(resolve => server.listen(0, '127.0.0.1', resolve));
  return `http://127.0.0.1:${(server.address() as AddressInfo).port}`;
}

function turn(base: string, init: RequestInit = {}): Promise<Response> {
  return fetch(`${base}/turn`, {
    method: 'POST',
    headers: { 'authorization': `Bearer ${TOKEN}`, 'content-type': 'application/json' },
    body: goldenBody,
    ...init,
  });
}

afterEach(() => {
  for (const s of servers.splice(0)) {
    s.closeAllConnections();
    s.close();
  }
});

describe('body cap', () => {
  it('allows 24 MiB, room for three 5 MiB images in base64 plus the rest of the turn', () => {
    expect(MAX_BODY_BYTES).toBe(24 * 1024 * 1024);
  });
});

describe('loadConfig', () => {
  it('needs a token of at least 32 bytes', () => {
    expect(() => loadConfig({})).toThrow('RUNTIME_TOKEN');
    expect(() => loadConfig({ RUNTIME_TOKEN: 'short' })).toThrow('RUNTIME_TOKEN');
  });

  it('reads the defaults and the overrides', () => {
    expect(loadConfig({ RUNTIME_TOKEN: TOKEN })).toEqual({ token: TOKEN, maxConcurrent: 8, port: 5160 });
    expect(loadConfig({ RUNTIME_TOKEN: TOKEN, RUNTIME_MAX_CONCURRENT: '2', PORT: '6000' })).toEqual({ token: TOKEN, maxConcurrent: 2, port: 6000 });
  });

  it('refuses a bad number', () => {
    expect(() => loadConfig({ RUNTIME_TOKEN: TOKEN, RUNTIME_MAX_CONCURRENT: '0' })).toThrow('RUNTIME_MAX_CONCURRENT');
    expect(() => loadConfig({ RUNTIME_TOKEN: TOKEN, PORT: 'x' })).toThrow('PORT');
  });
});

describe('runtime server', () => {
  it('answers /ping without auth', async () => {
    const base = await serve(async () => {});
    const res = await fetch(`${base}/ping`);
    expect(res.status).toBe(200);
    expect(await res.text()).toBe('ok');
  });

  it('answers 404 for another path and 405 for another method', async () => {
    const base = await serve(async () => {});
    expect((await fetch(`${base}/nope`)).status).toBe(404);
    expect((await fetch(`${base}/turn`)).status).toBe(405);
  });

  it('refuses a missing or wrong bearer with 401', async () => {
    const base = await serve(async () => {});
    expect((await turn(base, { headers: {} })).status).toBe(401);
    expect((await turn(base, { headers: { authorization: `Bearer ${'b'.repeat(32)}` } })).status).toBe(401);
    expect((await turn(base, { headers: { authorization: TOKEN } })).status).toBe(401);
  });

  it('refuses a body over the cap with 413 from the length header', async () => {
    const base = await serve(async () => {});
    const status = await new Promise<number>((resolve, reject) => {
      const req = httpRequest(`${base}/turn`, {
        method: 'POST',
        headers: { 'authorization': `Bearer ${TOKEN}`, 'content-length': String(MAX_BODY_BYTES + 1) },
      }, res => resolve(res.statusCode ?? 0));
      req.on('error', reject);
      req.flushHeaders();
    });
    expect(status).toBe(413);
  });

  it('refuses a streamed body that grows over the cap with 413', async () => {
    const base = await serve(async () => {});
    const body = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(new Uint8Array(MAX_BODY_BYTES));
        controller.enqueue(new Uint8Array(1));
        controller.close();
      },
    });
    const res = await turn(base, { body, duplex: 'half' } as RequestInit);
    expect(res.status).toBe(413);
  });

  it('refuses bad JSON and a bad request with 400 and field paths', async () => {
    const base = await serve(async () => {});
    const bad = await turn(base, { body: '{' });
    expect(bad.status).toBe(400);
    expect(await bad.json()).toEqual({ error: 'invalid_json' });
    const wrong = await turn(base, { body: JSON.stringify({ ...JSON.parse(goldenBody), contractVersion: 3 }) });
    expect(wrong.status).toBe(400);
    expect(await wrong.json()).toEqual({ error: 'invalid_request', fields: ['contractVersion'] });
  });

  it('streams each emitted event as one NDJSON line', async () => {
    const base = await serve(async (req, emit) => {
      emit({ type: 'RUN_STARTED', threadId: req.ids.thread, runId: req.ids.run });
      emit({ type: 'OAGENT_TERMINAL', status: 'completed' });
    });
    const res = await turn(base);
    expect(res.status).toBe(200);
    expect(res.headers.get('content-type')).toBe('application/x-ndjson');
    const lines = (await res.text()).trim().split('\n').map(l => JSON.parse(l) as Record<string, unknown>);
    expect(lines).toEqual([
      { type: 'RUN_STARTED', threadId: 't0193a2b4c5d67e8f9a0b1c2d3e4f5a6b', runId: 'Rn5Q8wYk2HhT3mVz7XcLp' },
      { type: 'OAGENT_TERMINAL', status: 'completed' },
    ]);
  });

  it('answers a refusal raised before the first event with its status and code', async () => {
    const base = await serve(async () => {
      throw new TurnRefusedError(422, 'snapshot_unloadable');
    });
    const res = await turn(base);
    expect(res.status).toBe(422);
    expect(await res.json()).toEqual({ error: 'snapshot_unloadable' });
  });

  it('answers 200 with an empty stream when the turn emits nothing', async () => {
    const base = await serve(async () => {});
    const res = await turn(base);
    expect(res.status).toBe(200);
    expect(await res.text()).toBe('');
  });

  it('waits for the client to drain before the next event', async () => {
    const big = 'x'.repeat(256 * 1024);
    let done = 0;
    let finished = false;
    const base = await serve(async (_req, emit) => {
      for (let i = 0; i < 64; i++) {
        await emit({ type: 'TEXT_MESSAGE_CONTENT', delta: big });
        done++;
      }
      finished = true;
    });
    const res = await new Promise<import('node:http').IncomingMessage>((resolve, reject) => {
      const req = httpRequest(`${base}/turn`, { method: 'POST', headers: { authorization: `Bearer ${TOKEN}` } }, resolve);
      req.on('error', reject);
      req.end(goldenBody);
    });
    res.pause();
    await new Promise(resolve => setTimeout(resolve, 200));
    expect(finished).toBe(false);
    expect(done).toBeLessThan(64);
    res.resume();
    await new Promise(resolve => res.on('end', resolve));
    expect(finished).toBe(true);
  });

  it('ends the stream when the turn throws', async () => {
    const base = await serve(async (_req, emit) => {
      emit({ type: 'RUN_STARTED' });
      throw new Error('boom');
    });
    const res = await turn(base);
    expect(await res.text()).toBe('{"type":"RUN_STARTED"}\n');
  });

  it('answers 503 at the concurrency cap and frees the slot after', async () => {
    let release = (): void => {};
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    const base = await serve(async (_req, emit) => {
      emit({ type: 'RUN_STARTED' });
      await held;
    }, 1);
    const first = await turn(base);
    expect(first.status).toBe(200);
    expect((await turn(base)).status).toBe(503);
    release();
    await first.text();
    const again = await turn(base);
    expect(again.status).toBe(200);
    await again.text();
  });

  it('aborts the turn when the client closes', async () => {
    let aborted: Promise<boolean> = Promise.resolve(false);
    const base = await serve(async (_req, emit, signal) => {
      aborted = new Promise(resolve => signal.addEventListener('abort', () => resolve(true)));
      emit({ type: 'RUN_STARTED' });
      await aborted;
    });
    const controller = new AbortController();
    const res = await turn(base, { signal: controller.signal });
    expect(res.status).toBe(200);
    controller.abort();
    await expect(aborted).resolves.toBe(true);
  });
});

describe('end to end', () => {
  it('streams one real turn as NDJSON, terminal last', async () => {
    const runTurn = createRunTurn({
      models: new ModelClients(),
      log: quiet,
      modelFor: () => new ScriptedModel([{ text: ['Hi, ', 'how can I help?'], usage: { inputTokens: 120, outputTokens: 9 } }]),
      openMcp: fakeMcp([]),
    });
    const base = await serve(runTurn);
    const res = await turn(base);
    expect(res.status).toBe(200);
    const lines = (await res.text()).trim().split('\n').map(l => JSON.parse(l) as Record<string, unknown>);
    expect(lines.map(l => l.type)).toEqual(['RUN_STARTED', 'TEXT_MESSAGE_START', 'TEXT_MESSAGE_CONTENT', 'TEXT_MESSAGE_CONTENT', 'TEXT_MESSAGE_END', 'RUN_FINISHED', 'OAGENT_TERMINAL']);
    expect(lines.at(-1)).toMatchObject({ status: 'completed', usage: { inputTokens: 120, outputTokens: 9 } });
  });
});

describe('early headers', () => {
  it('sends 200 and the NDJSON headers before the MCP servers finish opening', async () => {
    let release = (): void => {};
    const mcpOpen = new Promise<void>((resolve) => {
      release = resolve;
    });
    let mcpDone = false;
    const runTurn = createRunTurn({
      models: new ModelClients(),
      log: quiet,
      modelFor: () => new ScriptedModel([{ text: ['ok'] }]),
      openMcp: async () => {
        await mcpOpen;
        mcpDone = true;
        return fakeMcp([])();
      },
    });
    const base = await serve(runTurn);
    const res = await turn(base);
    expect(res.status).toBe(200);
    expect(res.headers.get('content-type')).toBe('application/x-ndjson');
    expect(mcpDone).toBe(false);
    release();
    const lines = (await res.text()).trim().split('\n').map(l => JSON.parse(l) as { type: string });
    expect(lines[0]!.type).toBe('RUN_STARTED');
    expect(lines.at(-1)!.type).toBe('OAGENT_TERMINAL');
  });
});

describe('start', () => {
  it('installs the guard, then listens with the configured runTurn', async () => {
    const lines: string[] = [];
    const probe = createServer().listen(0, '127.0.0.1');
    await new Promise(resolve => probe.once('listening', resolve));
    const port = (probe.address() as AddressInfo).port;
    await new Promise(resolve => probe.close(resolve));
    const server = await start({ RUNTIME_TOKEN: TOKEN, PORT: String(port), RUNTIME_ENV: 'development', RUNTIME_ALLOW_PRIVATE: `127.0.0.1:${port}` }, async (_req, emit) => {
      emit({ type: 'OAGENT_TERMINAL' });
    }, createLogger(line => lines.push(line)));
    servers.push(server);
    const base = `http://127.0.0.1:${(server.address() as AddressInfo).port}`;
    const res = await turn(base);
    expect(await res.text()).toBe('{"type":"OAGENT_TERMINAL"}\n');
    expect(lines.some(l => l.includes('"msg":"listening"'))).toBe(true);
  });
});
