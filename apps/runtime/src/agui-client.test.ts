import type { BaseEvent } from '@ag-ui/client';
import type { AddressInfo } from 'node:net';
import { Buffer } from 'node:buffer';
import { readdirSync, readFileSync } from 'node:fs';
import { createServer } from 'node:http';
import process from 'node:process';
import { fileURLToPath } from 'node:url';
import { HttpAgent } from '@ag-ui/client';
import { PROTOCOL_VERSION } from '@ag-ui/core';
import { RunAgentInputSchema } from '@ag-ui/core/schemas';
import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest';

const dir = fileURLToPath(new URL('../../../internal/dataplane/testdata/sse/', import.meta.url));
const CASES = ['cancelled', 'heartbeat_gap', 'interrupt_outcome', 'interrupted', 'plain_reply', 'raw_dropped', 'replay_settled', 'tool_call'];
const THREAD = 'client-thread-1';
const CHUNK = 7;

const streams = new Map<string, Buffer>();
const requests: { accept?: string, body: string }[] = [];
let base = '';
const server = createServer(async (req, res) => {
  const chunks: Buffer[] = [];
  for await (const chunk of req) {
    chunks.push(chunk as Buffer);
  }
  requests.push({ accept: req.headers.accept, body: Buffer.concat(chunks).toString('utf8') });
  const body = streams.get(req.url ?? '');
  if (req.method !== 'POST' || !body) {
    res.writeHead(404).end();
    return;
  }
  res.writeHead(200, { 'content-type': 'text/event-stream' });
  for (let i = 0; i < body.length; i += CHUNK) {
    res.write(body.subarray(i, i + CHUNK));
  }
  res.end();
});

beforeAll(async () => {
  await new Promise<void>(resolve => server.listen(0, '127.0.0.1', resolve));
  base = `http://127.0.0.1:${(server.address() as AddressInfo).port}`;
});

afterAll(async () => {
  await new Promise(resolve => server.close(resolve));
});

function dataEvents(sse: string): unknown[] {
  return sse.split('\n\n').flatMap(block => block.split('\n').filter(line => line.startsWith('data: ')).map(line => JSON.parse(line.slice(6)) as unknown));
}

async function runAgainst(path: string, sse: string): Promise<{ events: BaseEvent[], failed?: Error }> {
  streams.set(path, Buffer.from(sse));
  const agent = new HttpAgent({ url: `${base}${path}`, threadId: THREAD });
  const events: BaseEvent[] = [];
  let failed: Error | undefined;
  try {
    await agent.runAgent({}, {
      onEvent: ({ event }) => {
        events.push(event);
      },
      onRunFailed: ({ error }) => {
        failed = error;
      },
    });
  } catch (error) {
    failed ??= error as Error;
  }
  return { events, failed };
}

describe('ag-ui HttpAgent over the SSE relay goldens', () => {
  it('has a golden for every case and nothing else', () => {
    expect(readdirSync(dir).filter(f => f.endsWith('.sse')).map(f => f.slice(0, -4)).sort()).toEqual(CASES);
  });

  it.each(CASES)('reads %s and yields exactly its events', async (name) => {
    const sse = readFileSync(`${dir}${name}.sse`, 'utf8');
    const expected = dataEvents(sse);
    const { events, failed } = await runAgainst(`/${name}`, sse);

    expect(failed).toBeUndefined();
    expect(events).toEqual(expected);
    expect(events[0]).toMatchObject({ type: 'RUN_STARTED', threadId: THREAD });
    expect(['RUN_FINISHED', 'RUN_ERROR']).toContain(events.at(-1)?.type);
    expect(events.map(e => e.type)).not.toContain('RAW');
    expect(events.some(e => e.type.startsWith('OAGENT_'))).toBe(false);
  });

  it('posts only RunAgentInput fields, protocolVersion included, and asks for SSE', async () => {
    requests.length = 0;
    await runAgainst('/body', readFileSync(`${dir}plain_reply.sse`, 'utf8'));
    expect(requests).toHaveLength(1);
    expect(requests[0]!.accept).toBe('text/event-stream');
    const sent = JSON.parse(requests[0]!.body) as Record<string, unknown>;
    expect(sent).toMatchObject({ threadId: THREAD, protocolVersion: PROTOCOL_VERSION });
    expect(Object.keys(sent).filter(k => !(k in RunAgentInputSchema.shape))).toEqual([]);
  });

  it('refuses a stream that finishes with a text message still open', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {});
    const bad = [
      { type: 'RUN_STARTED', threadId: THREAD, runId: 'r1' },
      { type: 'TEXT_MESSAGE_START', messageId: 'm1', role: 'assistant' },
      { type: 'RUN_FINISHED', threadId: THREAD, runId: 'r1' },
    ].map((e, i) => `id: ${i + 1}\ndata: ${JSON.stringify(e)}\n\n`).join('');
    const { failed } = await runAgainst('/open_message', bad);
    expect(failed?.message).toMatch(/text messages are still active/);
    vi.restoreAllMocks();
  });
});

const live = {
  url: process.env.OAGENT_AGUI_LIVE_URL,
  key: process.env.OAGENT_AGUI_LIVE_KEY,
  agent: process.env.OAGENT_AGUI_LIVE_AGENT,
};

describe.skipIf(!live.url || !live.key || !live.agent)('ag-ui HttpAgent against a running server', () => {
  it('streams one run from RUN_STARTED to RUN_FINISHED', async () => {
    const threadId = `conformance-${Date.now()}`;
    const agent = new HttpAgent({
      url: `${live.url}/api/v1/agents/${live.agent}/runs`,
      headers: { authorization: `Bearer ${live.key}` },
      threadId,
      initialMessages: [{ id: 'u1', role: 'user', content: 'Reply with one short sentence.' }],
    });
    const events: BaseEvent[] = [];
    await agent.runAgent({}, { onEvent: ({ event }) => {
      events.push(event);
    } });

    expect(events[0]).toMatchObject({ type: 'RUN_STARTED', threadId });
    expect(events.at(-1)?.type).toBe('RUN_FINISHED');
    expect(events.map(e => e.type)).toContain('TEXT_MESSAGE_CONTENT');
  }, 120_000);
});
