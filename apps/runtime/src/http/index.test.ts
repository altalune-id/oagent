import type { Buffer } from 'node:buffer';
import type { AddressInfo } from 'node:net';
import type { RunTurn } from '../contract/index.ts';
import type { RuntimeServer } from './index.ts';
import { once } from 'node:events';
import { readFileSync } from 'node:fs';
import { createServer, request as httpRequest } from 'node:http';
import { connect } from 'node:net';
import { fileURLToPath } from 'node:url';

import { afterEach, describe, expect, it, vi } from 'vitest';
import { TurnRefusedError } from '../contract/index.ts';
import { createModels } from '../models/index.ts';
import { captureLogger } from '../test/logger.ts';
import { ScriptedModel } from '../test/scripted-model.ts';
import { fakeMcp } from '../test/turn-fixtures.ts';
import { createRunTurn } from '../turn/index.ts';
import { createRuntimeServer, listen, MAX_BODY_BYTES } from './index.ts';

const TOKEN = 'a'.repeat(32);
const goldenBody = readFileSync(fileURLToPath(new URL('../../../../internal/kernel/runtime/testdata/turn/plain_reply.request.json', import.meta.url)), 'utf8');
const quiet = captureLogger();
const runtimes: RuntimeServer[] = [];

async function serve(runTurn: RunTurn, maxConcurrent = 8): Promise<string> {
  const runtime = createRuntimeServer({ token: TOKEN, maxConcurrent, log: quiet, runTurn });
  runtimes.push(runtime);
  return `http://127.0.0.1:${await listen(runtime.server, 0)}`;
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
  for (const { server } of runtimes.splice(0)) {
    server.closeAllConnections();
    server.close();
  }
});

describe('body cap', () => {
  it('allows 24 MiB, room for three 5 MiB images in base64 plus the rest of the turn', () => {
    expect(MAX_BODY_BYTES).toBe(24 * 1024 * 1024);
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
      models: createModels(),
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
      models: createModels(),
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

describe('drain', () => {
  it('returns at once when no turn runs, and stops listening', async () => {
    const base = await serve(async () => {});
    await runtimes[0]!.drain(60_000);
    await expect(fetch(`${base}/ping`)).rejects.toThrow();
  });

  it('lets a running turn finish, and answers 503 to a request on an open connection', async () => {
    let release: (() => void) | undefined;
    const base = await serve(async (_req, emit) => {
      await new Promise<void>((resolve) => {
        release = resolve;
      });
      await emit({ type: 'OAGENT_TERMINAL' });
    });
    const inFlight = turn(base);
    await vi.waitFor(() => expect(release).toBeDefined());
    const sock = connect(Number(new URL(base).port), '127.0.0.1');
    await once(sock, 'connect');
    sock.write('GET /ping HTTP/1.1\r\nHost: runtime\r\n');
    await new Promise(resolve => setTimeout(resolve, 20));
    const drained = runtimes[0]!.drain(5_000);
    sock.write('\r\n');
    const [data] = await once(sock, 'data') as [Buffer];
    expect(data.toString()).toMatch(/^HTTP\/1\.1 503/);
    sock.destroy();
    release!();
    expect(await (await inFlight).text()).toBe('{"type":"OAGENT_TERMINAL"}\n');
    await drained;
  });

  it('cuts a turn still running when the grace period ends, with no terminal event', async () => {
    let aborted = false;
    const base = await serve(async (_req, emit, signal) => {
      await emit({ type: 'RUN_STARTED' });
      await new Promise<void>((resolve) => {
        signal.addEventListener('abort', () => {
          aborted = true;
          resolve();
        });
      });
      await emit({ type: 'OAGENT_TERMINAL' });
    });
    const res = await turn(base);
    await runtimes[0]!.drain(50);
    const text = await res.text().catch(() => '');
    expect(text).not.toContain('OAGENT_TERMINAL');
    await vi.waitFor(() => expect(aborted).toBe(true));
  });
});

describe('listen', () => {
  it('rejects when the port is taken', async () => {
    const taken = createServer().listen(0);
    await once(taken, 'listening');
    const port = (taken.address() as AddressInfo).port;
    await expect(listen(createServer(), port)).rejects.toMatchObject({ code: 'EADDRINUSE' });
    taken.close();
  });
});
