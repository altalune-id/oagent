import type { BaseEvent, RunAgentInput } from '@ag-ui/client';
import type { AddressInfo } from 'node:net';
import { Buffer } from 'node:buffer';
import { readdirSync, readFileSync } from 'node:fs';
import { createServer } from 'node:http';
import process from 'node:process';
import { fileURLToPath } from 'node:url';
import { buildResumeArray, getRunOutcome, HttpAgent } from '@ag-ui/client';
import { PROTOCOL_VERSION } from '@ag-ui/core';
import { InterruptSchema, RunAgentInputSchema, RunFinishedEventSchema } from '@ag-ui/core/schemas';
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from 'vitest';

const dir = fileURLToPath(new URL('../../../internal/dataplane/testdata/sse/', import.meta.url));
const CASES = ['cancelled', 'heartbeat_gap', 'interrupt_outcome', 'interrupted', 'plain_reply', 'raw_dropped', 'replay_settled', 'tool_call'];
const RESUME_ENTRY_KEYS = ['interruptId', 'status', 'payload', 'metadata'];
const RESUME_PAYLOAD_KEYS = ['approved', 'text'];

type Decoded = { want: 'ok', approved: boolean } | { want: 'APR004', missing: string[] } | { want: 'APR005' } | { want: 'invalid', field: string };
type ResumeVector = { name: string, open: string[], resume: unknown[], want: Record<string, unknown> };

class JsonObject {
  readonly pairs: [string, unknown][];

  constructor(pairs: [string, unknown][]) {
    this.pairs = pairs;
  }

  get(key: string): unknown {
    return this.pairs.find(([k]) => k === key)?.[1];
  }

  has(key: string): boolean {
    return this.pairs.some(([k]) => k === key);
  }

  duplicated(): boolean {
    return new Set(this.pairs.map(([k]) => k)).size !== this.pairs.length;
  }
}

function parseKeepingDuplicates(text: string): unknown {
  let i = 0;
  const space = () => {
    while (/\s/.test(text[i] ?? '')) {
      i++;
    }
  };
  const value = (): unknown => {
    space();
    const c = text[i];
    if (c === '{') {
      i++;
      const pairs: [string, unknown][] = [];
      space();
      if (text[i] === '}') {
        i++;
        return new JsonObject(pairs);
      }
      for (;;) {
        space();
        const key = value() as string;
        space();
        i++;
        pairs.push([key, value()]);
        space();
        if (text[i++] === '}') {
          return new JsonObject(pairs);
        }
      }
    }
    if (c === '[') {
      i++;
      const items: unknown[] = [];
      space();
      if (text[i] === ']') {
        i++;
        return items;
      }
      for (;;) {
        items.push(value());
        space();
        if (text[i++] === ']') {
          return items;
        }
      }
    }
    const token = /^(?:"(?:[^"\\]|\\.)*"|-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?|true|false|null)/.exec(text.slice(i));
    if (!token) {
      throw new Error(`bad JSON at ${i}`);
    }
    i += token[0].length;
    return JSON.parse(token[0]) as unknown;
  };
  return value();
}

function plain(v: unknown): unknown {
  if (v instanceof JsonObject) {
    return Object.fromEntries(v.pairs.map(([k, x]) => [k, plain(x)]));
  }
  return Array.isArray(v) ? v.map(plain) : v;
}

const resumeVectors = ((parseKeepingDuplicates(readFileSync(fileURLToPath(new URL('../../../internal/dataplane/testdata/resume_vectors.json', import.meta.url)), 'utf8')) as JsonObject).get('vectors') as JsonObject[]).map((v): ResumeVector => {
  const { name, open, resume: _, ...want } = plain(v) as Record<string, unknown>;
  return { name: name as string, open: open as string[], resume: v.get('resume') as unknown[], want };
});
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

afterEach(() => {
  vi.useRealTimers();
});

afterAll(async () => {
  await new Promise(resolve => server.close(resolve));
});

function dataEvents(sse: string): unknown[] {
  return sse.split('\n\n').flatMap(block => block.split('\n').filter(line => line.startsWith('data: ')).map(line => JSON.parse(line.slice(6)) as unknown));
}

function sseOf(events: unknown[]): string {
  return events.map((e, i) => `id: ${i + 1}\ndata: ${JSON.stringify(e)}\n\n`).join('');
}

function byteOrder(keys: string[]): string[] {
  return [...keys].sort((a, b) => Buffer.compare(Buffer.from(a, 'utf8'), Buffer.from(b, 'utf8')));
}

function decodeAnswer(payload: unknown): Decoded | boolean {
  if (!(payload instanceof JsonObject) || payload.duplicated()) {
    return { want: 'invalid', field: 'resume.payload' };
  }
  let ok: boolean | undefined;
  for (const key of byteOrder(payload.pairs.map(([k]) => k))) {
    if (!RESUME_PAYLOAD_KEYS.includes(key)) {
      return { want: 'APR005' };
    }
    const value = payload.get(key);
    if (key === 'approved') {
      if (typeof value !== 'boolean') {
        return { want: 'invalid', field: 'resume.payload.approved' };
      }
      ok = value;
    }
  }
  if (ok === undefined) {
    return { want: 'invalid', field: 'resume.payload.approved' };
  }
  if (payload.has('text') && typeof payload.get('text') !== 'string') {
    return { want: 'invalid', field: 'resume.payload.text' };
  }
  return ok;
}

function decodeResume(open: string[], resume: unknown[]): Decoded {
  const seen = new Set<string>();
  let approved = true;
  for (const entry of resume) {
    if (!(entry instanceof JsonObject) || entry.duplicated()) {
      return { want: 'invalid', field: 'resume' };
    }
    for (const key of byteOrder(entry.pairs.map(([k]) => k))) {
      if (!RESUME_ENTRY_KEYS.includes(key)) {
        return { want: 'invalid', field: `resume.${key}` };
      }
      if ((key === 'interruptId' || key === 'status') && typeof entry.get(key) !== 'string') {
        return { want: 'invalid', field: `resume.${key}` };
      }
    }
    const id = entry.get('interruptId');
    if (typeof id !== 'string' || id === '') {
      return { want: 'invalid', field: 'resume.interruptId' };
    }
    const status = entry.get('status');
    if (status !== 'resolved' && status !== 'cancelled') {
      return { want: 'invalid', field: 'resume.status' };
    }
    if (!open.includes(id) || seen.has(id)) {
      return { want: 'invalid', field: 'resume.interruptId' };
    }
    seen.add(id);
    if (status === 'cancelled') {
      approved = false;
      continue;
    }
    const answer = decodeAnswer(entry.get('payload'));
    if (typeof answer !== 'boolean') {
      return answer;
    }
    approved &&= answer;
  }
  const missing = open.filter(id => !seen.has(id));
  return missing.length > 0 ? { want: 'APR004', missing } : { want: 'ok', approved };
}

async function runAgainst(path: string, sse: string, agent = new HttpAgent({ url: `${base}${path}`, threadId: THREAD }), parameters = {}): Promise<{ events: BaseEvent[], failed?: Error }> {
  streams.set(path, Buffer.from(sse));
  const events: BaseEvent[] = [];
  let failed: Error | undefined;
  try {
    await agent.runAgent(parameters, {
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

  it('reads interrupt_outcome as an interrupt outcome that passes core\'s schema', async () => {
    const { events, failed } = await runAgainst('/interrupt', readFileSync(`${dir}interrupt_outcome.sse`, 'utf8'));
    expect(failed).toBeUndefined();
    const finished = RunFinishedEventSchema.parse(events.at(-1));
    const outcome = getRunOutcome(finished);
    expect(outcome?.type).toBe('interrupt');
    const interrupts = outcome?.type === 'interrupt' ? outcome.interrupts : [];
    expect(interrupts.length).toBeGreaterThan(0);
    for (const i of interrupts) {
      expect(InterruptSchema.safeParse(i).success).toBe(true);
      expect(i.id).not.toMatch(/^hook:/);
    }
  });

  it('resumes with a stock resume[] POST that meets the decode rules', async () => {
    const agent = new HttpAgent({ url: `${base}/resume`, threadId: THREAD });
    const first = await runAgainst('/resume', readFileSync(`${dir}interrupt_outcome.sse`, 'utf8'), agent);
    expect(first.failed).toBeUndefined();
    const outcome = getRunOutcome(RunFinishedEventSchema.parse(first.events.at(-1)));
    const interrupts = outcome?.type === 'interrupt' ? outcome.interrupts : [];
    const openIds = interrupts.map(i => i.id);
    expect(openIds.length).toBeGreaterThan(0);
    const expiries = interrupts.flatMap(i => (i.expiresAt ? [Date.parse(i.expiresAt)] : []));
    if (expiries.length > 0) {
      vi.setSystemTime(Math.min(...expiries) - 60_000);
    }

    const resume = buildResumeArray(interrupts, Object.fromEntries(openIds.map(id => [id, { status: 'resolved' as const, payload: { approved: true, text: 'fine' } }])));
    requests.length = 0;
    const second = await runAgainst('/resume', readFileSync(`${dir}plain_reply.sse`, 'utf8'), agent, { resume });
    expect(second.failed).toBeUndefined();

    expect(requests).toHaveLength(1);
    const sent = JSON.parse(requests[0]!.body) as RunAgentInput;
    expect(Object.keys(sent).filter(k => !(k in RunAgentInputSchema.shape))).toEqual([]);
    const raw = parseKeepingDuplicates(requests[0]!.body) as JsonObject;
    expect(decodeResume(openIds, (raw.get('resume') ?? []) as unknown[])).toEqual({ want: 'ok', approved: true });
  });

  it('reads every shared resume vector, duplicate keys included', () => {
    expect(resumeVectors.length).toBeGreaterThanOrEqual(21);
  });

  it('checks keys in UTF-8 byte order, as Go does, not UTF-16 order', () => {
    const halfwidth = '\uFF61';
    const emoji = '\u{1F600}';
    expect([emoji, halfwidth].sort()).toEqual([emoji, halfwidth]);
    expect(byteOrder([emoji, halfwidth])).toEqual([halfwidth, emoji]);
    const entry = new JsonObject([['interruptId', 'call_1'], ['status', 'cancelled'], [emoji, 1], [halfwidth, 1]]);
    expect(decodeResume(['call_1'], [entry])).toEqual({ want: 'invalid', field: `resume.${halfwidth}` });
  });

  it.each(resumeVectors.map(v => [v.name, v] as const))('decodes the shared resume vector: %s', (_name, v) => {
    expect(decodeResume(v.open, v.resume)).toEqual(v.want);
  });

  it('refuses a stream that finishes with a text message still open', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {});
    const bad = sseOf([
      { type: 'RUN_STARTED', threadId: THREAD, runId: 'r1' },
      { type: 'TEXT_MESSAGE_START', messageId: 'm1', role: 'assistant' },
      { type: 'RUN_FINISHED', threadId: THREAD, runId: 'r1' },
    ]);
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
