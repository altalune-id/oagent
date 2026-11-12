import { existsSync, readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import process from 'node:process';
import { describe, expect, it } from 'vitest';
import { readEvents } from './sse';

const goldens = join(process.cwd(), '../../internal/dataplane/testdata/sse');

function streamOf(text: string, chunk = 7): ReadableStream<Uint8Array> {
  const bytes = new TextEncoder().encode(text);
  let at = 0;
  return new ReadableStream({
    pull(c) {
      if (at >= bytes.length) {
        c.close();
        return;
      }
      c.enqueue(bytes.slice(at, at + chunk));
      at += chunk;
    },
  });
}

const interruptGolden = join(goldens, 'interrupt_outcome.sse');

const interrupt = {
  id: 'call_1',
  reason: 'tool_call',
  toolCallId: 'call_1',
  message: 'yasaku · record_expense\namount: 25000\naccount: cash',
  expiresAt: '2026-11-07T00:00:00Z',
  responseSchema: { type: 'object', properties: { approved: { type: 'boolean' }, text: { type: 'string' } }, required: ['approved'] },
  metadata: { approval: 'Ap4Q8wYk2HhT3mVz7XcLp' },
};

const interruptStream = [
  { type: 'RUN_STARTED', threadId: 'client-thread-1', runId: 'Rn5Q8wYk2HhT3mVz7XcLp' },
  { type: 'TOOL_CALL_START', toolCallId: 'call_1', toolCallName: 'yasaku__record_expense', parentMessageId: 'msg_asst_1' },
  { type: 'TOOL_CALL_ARGS', toolCallId: 'call_1', delta: '{"amount":25000,"account":"cash","confirm":true}' },
  { type: 'TOOL_CALL_END', toolCallId: 'call_1' },
  { type: 'RUN_FINISHED', threadId: 'client-thread-1', runId: 'Rn5Q8wYk2HhT3mVz7XcLp', outcome: { type: 'interrupt', interrupts: [interrupt] } },
].map((e, i) => `id: ${i + 1}\ndata: ${JSON.stringify(e)}\n\n`).join('');

function dataOf(text: string) {
  return text.split('\n').filter(l => l.startsWith('data: ')).map(l => JSON.parse(l.slice(6)) as Record<string, unknown>);
}

async function expectInterruptOutcome(text: string) {
  const expected = dataOf(text);
  expect(expected.at(-1)).toMatchObject({ type: 'RUN_FINISHED', outcome: { type: 'interrupt' } });
  for (const chunk of [1, 7, 64]) {
    const events = await all(text, chunk);
    expect(events).toEqual(expected);
    expect(events.at(-1)!.outcome).toEqual(expected.at(-1)!.outcome);
  }
}

async function all(text: string, chunk?: number) {
  const out = [];
  for await (const e of readEvents(streamOf(text, chunk))) {
    out.push(e);
  }
  return out;
}

describe('the AG-UI SSE reader', () => {
  const files = readdirSync(goldens).filter(f => f.endsWith('.sse'));

  it('has the data plane goldens to read', () => {
    expect(files.length).toBeGreaterThan(5);
  });

  it.each(files)('reads every event of %s in order, whatever the chunk size', async (file) => {
    const text = readFileSync(join(goldens, file), 'utf8');
    const expected = text.split('\n').filter(l => l.startsWith('data: ')).map(l => JSON.parse(l.slice(6)));
    for (const chunk of [1, 5, 64, 1 << 16]) {
      expect(await all(text, chunk)).toEqual(expected);
    }
  });

  it('keeps RUN_FINISHED\'s interrupt outcome whole, in 1- and 7-byte chunks (the runtime\'s interrupt stream)', async () => {
    await expectInterruptOutcome(interruptStream);
  });

  it.skipIf(!existsSync(interruptGolden))('keeps RUN_FINISHED\'s interrupt outcome whole in the data plane golden interrupt_outcome.sse (runs once it ships)', async () => {
    await expectInterruptOutcome(readFileSync(interruptGolden, 'utf8'));
  });

  it('drops comments, unknown types, broken frames and an unfinished last frame', async () => {
    const text = [
      ': ping',
      '',
      'data: {"type":"RAW","event":{}}',
      '',
      'data: {"type":"RUN_STARTED","runId":"r"}',
      '',
      'data: {not json',
      '',
      'data: {"type":"TEXT_MESSAGE_CONTENT",',
      'data: "messageId":"m","delta":"a"}',
      '',
      'id: 9\r\ndata: {"type":"RUN_FINISHED"}\r\n\r\n',
      'data: {"type":"RUN_ERROR"}',
    ].join('\n');
    expect(await all(text)).toEqual([
      { type: 'RUN_STARTED', runId: 'r' },
      { type: 'TEXT_MESSAGE_CONTENT', messageId: 'm', delta: 'a' },
      { type: 'RUN_FINISHED' },
    ]);
  });
});
