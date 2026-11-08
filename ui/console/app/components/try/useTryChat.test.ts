import type { Mock } from 'vitest';
import type { MessagesPage, TryApi } from './useTryChat';
import { create } from '@bufbuild/protobuf';
import { describe, expect, it, vi } from 'vitest';
import { effectScope } from 'vue';
import { errorCode, errorMeta } from '#base/lib/apperror/message';
import { RunSchema, RunStatus } from '#gen/thread/v1/thread_pb';
import { phaseLabel } from './phase';
import { envelopeError, tryApi, useTryChat } from './useTryChat';

const runId = 'Rn2wQ8mX4vZ9tB3nC7kPq';

function sse(events: Record<string, unknown>[], open = false) {
  const text = events.map(e => `data: ${JSON.stringify(e)}\n\n`).join('');
  let push: ((e: Record<string, unknown>) => void) | undefined;
  let close: (() => void) | undefined;
  const body = new ReadableStream<Uint8Array>({
    start(c) {
      c.enqueue(new TextEncoder().encode(text));
      push = e => c.enqueue(new TextEncoder().encode(`data: ${JSON.stringify(e)}\n\n`));
      close = () => c.close();
      if (!open) {
        c.close();
      }
    },
  });
  return { body, push: (e: Record<string, unknown>) => push!(e), close: () => close!() };
}

function reply(text: string) {
  return [
    { type: 'RUN_STARTED', runId },
    { type: 'TOOL_CALL_START', toolCallId: 'c1', toolCallName: 'yasaku__list_wallets' },
    { type: 'TOOL_CALL_ARGS', toolCallId: 'c1', delta: '{"limit":' },
    { type: 'TOOL_CALL_ARGS', toolCallId: 'c1', delta: '3}' },
    { type: 'TOOL_CALL_END', toolCallId: 'c1' },
    { type: 'TOOL_CALL_RESULT', toolCallId: 'c1', content: '[{"name":"Cash"}]' },
    { type: 'TEXT_MESSAGE_START', messageId: 'm1' },
    { type: 'TEXT_MESSAGE_CONTENT', messageId: 'm1', delta: text.slice(0, 4) },
    { type: 'TEXT_MESSAGE_CONTENT', messageId: 'm1', delta: text.slice(4) },
    { type: 'TEXT_MESSAGE_END', messageId: 'm1' },
    { type: 'RUN_FINISHED', runId },
  ];
}

const runPB = (status = RunStatus.COMPLETED, over = {}) => create(RunSchema, { id: runId, threadId: 'Th7sK2pQ9mXr4StV8nBcW', status, agentVersionKind: 'draft', usage: { inputTokens: 100n, outputTokens: 20n }, costMicros: 1500n, ...over });

type FakeApi = { [K in keyof TryApi]: Mock<TryApi[K]> };

function fake(over: Partial<Record<keyof TryApi, Mock>> = {}) {
  const api = {
    run: vi.fn<TryApi['run']>().mockImplementation(async () => sse(reply('Siap, dicatat.')).body),
    cancel: vi.fn<TryApi['cancel']>().mockResolvedValue(),
    messages: vi.fn<TryApi['messages']>().mockResolvedValue({ messages: [], next_after: 0 }),
    getRun: vi.fn<TryApi['getRun']>().mockResolvedValue(runPB()),
    ...over,
  } as FakeApi;
  let n = 0;
  const chat = useTryChat(api, () => `id-${++n}`);
  return { api, chat };
}

describe('try chat', () => {
  it('streams a turn into the transcript: the person, the tool call and the reply, then the run row with its usage', async () => {
    const { api, chat } = fake();
    expect(chat.threadId.value).toBe('id-1');
    await chat.send('  catat kopi 40rb  ');
    expect(api.run.mock.calls[0]![0]).toEqual({ threadId: 'id-1', messages: [{ id: 'id-2', role: 'user', content: 'catat kopi 40rb' }] });
    expect(chat.items.value.map(i => i.kind)).toEqual(['user', 'tool', 'agent', 'run']);
    const [, tool, agent, run] = chat.items.value;
    expect(tool).toMatchObject({ name: 'yasaku__list_wallets', args: { limit: 3 }, result: [{ name: 'Cash' }], state: 'done' });
    expect(agent).toMatchObject({ text: 'Siap, dicatat.', streaming: false });
    expect(run).toMatchObject({ run: { id: runId, draft: true, tokens: 120, costCents: 0.15 } });
    expect(api.getRun).toHaveBeenCalledWith(runId);
    expect([chat.phase.value, chat.tokens.value, chat.costCents.value, chat.busy.value]).toEqual(['completed', 120, 0.15, false]);
    expect(chat.toolCalls.value).toHaveLength(1);
    await chat.send('lagi');
    expect([chat.tokens.value, chat.costCents.value]).toEqual([240, 0.3]);
    expect(chat.items.value.filter(i => i.kind === 'agent').map(i => i.kind === 'agent' && i.text)).toEqual(['Siap, dicatat.', 'Siap, dicatat.']);
    expect(chat.toolCalls.value).toHaveLength(2);
    expect(api.run.mock.calls[1]![0].threadId).toBe('id-1');
  });

  it('shows the phase while it streams, and ignores a blank message or one sent while busy', async () => {
    const s = sse([{ type: 'RUN_STARTED', runId }], true);
    const { api, chat } = fake({ run: vi.fn().mockResolvedValue(s.body) });
    await chat.send('   ');
    expect(api.run).not.toHaveBeenCalled();
    const sent = chat.send('halo');
    await vi.waitFor(() => expect(chat.phase.value).toBe('thinking'));
    s.push({ type: 'TOOL_CALL_START', toolCallId: 'c1', toolCallName: 't' });
    await vi.waitFor(() => expect(chat.phase.value).toBe('tool'));
    s.push({ type: 'TEXT_MESSAGE_CONTENT', messageId: 'm1', delta: 'Ha' });
    await vi.waitFor(() => expect(chat.phase.value).toBe('streaming'));
    expect(chat.items.value.at(-1)).toMatchObject({ kind: 'agent', text: 'Ha', streaming: true });
    await chat.send('again');
    expect(api.run).toHaveBeenCalledTimes(1);
    s.push({ type: 'RUN_FINISHED', runId });
    s.close();
    await sent;
    expect(chat.phase.value).toBe('completed');
  });

  it('shows a refused turn as an error with no run, and Retry sends it again under a new message id', async () => {
    const refused = new Error('refused');
    const { api, chat } = fake({ run: vi.fn().mockRejectedValueOnce(refused).mockImplementation(async () => sse(reply('ok')).body) });
    await chat.send('halo');
    expect([chat.phase.value, chat.error.value]).toEqual(['failed', refused]);
    expect(chat.items.value.map(i => i.kind)).toEqual(['user']);
    await chat.retry();
    expect(api.run.mock.calls[1]![0].messages[0]).toEqual({ id: 'id-3', role: 'user', content: 'halo' });
    expect(chat.error.value).toBeNull();
    expect(chat.items.value.filter(i => i.kind === 'user')).toHaveLength(1);
    expect(chat.phase.value).toBe('completed');
  });

  it('marks a failed run with the failed bubble and its run row, and Retry clears the bubble', async () => {
    const failing = [{ type: 'RUN_STARTED', runId }, { type: 'TOOL_CALL_START', toolCallId: 'c1', toolCallName: 't' }, { type: 'RUN_ERROR', code: 'runtime_error' }];
    const { chat } = fake({ run: vi.fn().mockResolvedValueOnce(sse(failing).body).mockImplementation(async () => sse(reply('ok')).body), getRun: vi.fn().mockResolvedValueOnce(runPB(RunStatus.FAILED, { errorCode: 'RUN004' })).mockResolvedValue(runPB()) });
    await chat.send('halo');
    expect(chat.phase.value).toBe('failed');
    expect(chat.items.value.map(i => (i.kind === 'agent' ? `agent:${!!i.failed}` : i.kind === 'tool' ? `tool:${i.state}` : i.kind))).toEqual(['user', 'tool:failed', 'agent:true', 'run']);
    await chat.retry();
    expect(chat.items.value.some(i => i.kind === 'agent' && i.failed)).toBe(false);
    expect(chat.phase.value).toBe('completed');
    await chat.retry();
    expect(chat.phase.value).toBe('completed');
  });

  it('names a stopped run and one that hit its limit by the run status, falling back to the event when GetRun fails', async () => {
    const cancelled = [{ type: 'RUN_STARTED', runId }, { type: 'RUN_ERROR', code: 'cancelled' }];
    const a = fake({ run: vi.fn().mockResolvedValue(sse(cancelled).body), getRun: vi.fn().mockResolvedValue(runPB(RunStatus.CANCELLED)) });
    await a.chat.send('halo');
    expect([a.chat.phase.value, a.chat.items.value.some(i => i.kind === 'notice' && i.key === 'try.stopped')]).toEqual(['cancelled', false]);
    const b = fake({ getRun: vi.fn().mockResolvedValue(runPB(RunStatus.LIMIT_REACHED)) });
    await b.chat.send('halo');
    expect(b.chat.phase.value).toBe('limit');
    const c = fake({ run: vi.fn().mockResolvedValue(sse(cancelled).body), getRun: vi.fn().mockRejectedValue(new Error('offline')) });
    await c.chat.send('halo');
    expect([c.chat.phase.value, c.chat.items.value.some(i => i.kind === 'run')]).toEqual(['cancelled', false]);
    expect(phaseLabel('limit').key).toBe('runstatus.limit_reached');
  });

  it('shows Stopping after Stop, and settles on the run\'s real outcome: completed keeps the reply and says nothing about stopping', async () => {
    const s = sse([{ type: 'RUN_STARTED', runId }], true);
    const { api, chat } = fake({ run: vi.fn().mockResolvedValue(s.body) });
    const sent = chat.send('halo');
    await vi.waitFor(() => expect(chat.canStop.value).toBe(true));
    await chat.stop();
    expect(api.cancel).toHaveBeenCalledWith(runId);
    expect([chat.phase.value, chat.busy.value, chat.canStop.value]).toEqual(['stopping', true, false]);
    s.push({ type: 'TEXT_MESSAGE_CONTENT', messageId: 'm1', delta: 'Siap.' });
    await vi.waitFor(() => expect(chat.items.value.some(i => i.kind === 'agent')).toBe(true));
    expect(chat.phase.value).toBe('stopping');
    s.push({ type: 'RUN_FINISHED', runId });
    s.close();
    await sent;
    expect(chat.phase.value).toBe('completed');
    expect(chat.items.value.some(i => i.kind === 'notice' && i.key === 'try.stopped')).toBe(false);
    expect(chat.items.value.find(i => i.kind === 'agent')).toMatchObject({ text: 'Siap.' });
  });

  it('says Stopped by you only when the run itself ended cancelled', async () => {
    const s = sse([{ type: 'RUN_STARTED', runId }], true);
    const { chat } = fake({ run: vi.fn().mockResolvedValue(s.body), getRun: vi.fn().mockResolvedValue(runPB(RunStatus.CANCELLED)) });
    const sent = chat.send('halo');
    await vi.waitFor(() => expect(chat.canStop.value).toBe(true));
    await chat.stop();
    s.push({ type: 'RUN_ERROR', code: 'cancelled' });
    s.close();
    await sent;
    expect(chat.phase.value).toBe('cancelled');
    expect(chat.items.value.filter(i => i.kind === 'notice').map(i => i.kind === 'notice' && i.key)).toEqual(['try.stopped']);
  });

  it('cannot stop before the run has started, and a failed cancel keeps the run going with the error shown', async () => {
    const pending = fake({ run: vi.fn(() => new Promise<ReadableStream<Uint8Array>>(() => {})) });
    void pending.chat.send('halo');
    expect([pending.chat.busy.value, pending.chat.canStop.value]).toEqual([true, false]);
    await pending.chat.stop();
    expect([pending.api.cancel.mock.calls.length, pending.chat.phase.value]).toEqual([0, 'thinking']);

    const s = sse([{ type: 'RUN_STARTED', runId }, { type: 'TEXT_MESSAGE_CONTENT', messageId: 'm1', delta: 'a' }], true);
    const offline = new Error('offline');
    const failing = fake({ run: vi.fn().mockResolvedValue(s.body), cancel: vi.fn().mockRejectedValue(offline) });
    void failing.chat.send('halo');
    await vi.waitFor(() => expect(failing.chat.phase.value).toBe('streaming'));
    await failing.chat.stop();
    expect([failing.chat.phase.value, failing.chat.error.value, failing.chat.canStop.value]).toEqual(['streaming', offline, true]);
  });

  it('says the stream dropped, and Reload rebuilds the reply from the stored messages and reads the run', async () => {
    const dropped = [{ type: 'RUN_STARTED', runId }, { type: 'TEXT_MESSAGE_CONTENT', messageId: 'm1', delta: 'Sia' }];
    const pages: MessagesPage[] = [
      { messages: [{ seq: 2, message: { role: 'assistant', content: '', toolCalls: [{ id: 'c1', function: { name: 'yasaku__list_wallets', arguments: '{"limit":3}' } }] } }, { seq: 1, message: { role: 'user', content: 'halo' } }], next_after: 2 },
      { messages: [{ seq: 3, message: { role: 'tool', toolCallId: 'c1', content: [{ type: 'text', text: '[1]' }] } }, { seq: 4, message: { role: 'assistant', content: 'Siap.' } }], next_after: 4 },
      { messages: [], next_after: 4 },
    ];
    const messages = vi.fn<TryApi['messages']>();
    pages.forEach(p => messages.mockResolvedValueOnce(p));
    const { chat } = fake({ run: vi.fn().mockResolvedValue(sse(dropped).body), messages });
    await chat.send('halo');
    expect(chat.phase.value).toBe('dropped');
    expect(chat.items.value.at(-1)).toMatchObject({ kind: 'notice', key: 'try.dropped' });
    await chat.reload();
    expect(messages.mock.calls.map(c => c[1])).toEqual([0, 2, 4]);
    expect(chat.items.value.map(i => i.kind)).toEqual(['user', 'tool', 'agent', 'run']);
    expect(chat.items.value[1]).toMatchObject({ args: { limit: 3 }, result: [1], state: 'done' });
    expect(chat.items.value[2]).toMatchObject({ text: 'Siap.' });
    expect(chat.phase.value).toBe('completed');
  });

  it('keeps a still-running run dropped after Reload, and shows a failed reload as an error', async () => {
    const dropped = [{ type: 'RUN_STARTED', runId }, { type: 'RUN_ERROR', code: 'detached' }];
    const { chat } = fake({ run: vi.fn().mockResolvedValue(sse(dropped).body), getRun: vi.fn().mockResolvedValue(runPB(RunStatus.RUNNING)), messages: vi.fn().mockResolvedValueOnce({ messages: [], next_after: 0 }).mockRejectedValueOnce(new Error('offline')) });
    await chat.send('halo');
    await chat.reload();
    expect(chat.phase.value).toBe('dropped');
    expect(chat.items.value.map(i => i.kind)).toEqual(['user', 'notice']);
    await chat.reload();
    expect(chat.error.value).toEqual(new Error('offline'));
  });

  it('starts a new chat on a new thread and drops what the old stream still sends', async () => {
    const s = sse([{ type: 'RUN_STARTED', runId }], true);
    const { chat } = fake({ run: vi.fn().mockResolvedValue(s.body) });
    const sent = chat.send('halo');
    await vi.waitFor(() => expect(chat.busy.value).toBe(true));
    chat.newChat();
    s.push({ type: 'TEXT_MESSAGE_CONTENT', messageId: 'm1', delta: 'late' });
    s.close();
    await sent;
    expect([chat.items.value.length, chat.phase.value, chat.threadId.value]).toEqual([0, 'idle', 'id-3']);
  });

  it('stops listening when its scope ends', async () => {
    const s = sse([{ type: 'RUN_STARTED', runId }], true);
    const scope = effectScope();
    const { chat } = scope.run(() => fake({ run: vi.fn().mockResolvedValue(s.body) }))!;
    const sent = chat.send('halo');
    await vi.waitFor(() => expect(chat.busy.value).toBe(true));
    scope.stop();
    s.push({ type: 'TEXT_MESSAGE_CONTENT', messageId: 'm1', delta: 'late' });
    s.close();
    await sent;
    expect(chat.items.value.some(i => i.kind === 'agent')).toBe(false);
  });
});

describe('try chat memory', () => {
  const stored: MessagesPage = { messages: [
    { seq: 1, message: { role: 'user', content: 'halo' } },
    { seq: 2, message: { role: 'assistant', content: '', toolCalls: [{ id: 'c1', function: { name: 'yasaku__list_wallets', arguments: '{}' } }] } },
    { seq: 3, message: { role: 'tool', toolCallId: 'c1', content: '[1]' } },
    { seq: 4, message: { role: 'assistant', content: 'Siap.' } },
  ], next_after: 4 };

  function remembered(id: string | null) {
    let kept = id;
    return { get: vi.fn(() => kept), set: vi.fn((v: string) => {
      kept = v;
    }) };
  }

  it('restores the remembered conversation without times, then keeps talking on the same thread', async () => {
    const memory = remembered('thread-old');
    const messages = vi.fn<TryApi['messages']>().mockResolvedValueOnce(stored).mockResolvedValue({ messages: [], next_after: 4 });
    const api = { run: vi.fn().mockImplementation(async () => sse(reply('ok')).body), cancel: vi.fn(), messages, getRun: vi.fn().mockResolvedValue(runPB()) } as unknown as TryApi;
    const chat = useTryChat(api, () => 'new-id', memory);
    expect(chat.threadId.value).toBe('thread-old');
    await chat.restore();
    expect(messages.mock.calls[0]).toEqual(['thread-old', 0]);
    expect(chat.items.value.map(i => i.kind)).toEqual(['user', 'tool', 'agent']);
    expect(chat.items.value[0]).toMatchObject({ text: 'halo', at: undefined });
    expect(chat.items.value[1]).toMatchObject({ result: [1], state: 'done' });
    expect(chat.phase.value).toBe('idle');
    await chat.restore();
    expect(messages).toHaveBeenCalledTimes(2);
    await chat.send('lagi');
    expect((api.run as ReturnType<typeof vi.fn>).mock.calls[0]![0].threadId).toBe('thread-old');
  });

  it('starts fresh with nothing remembered or when the remembered thread cannot be read, and New chat remembers the new thread', async () => {
    const none = remembered(null);
    const api = { run: vi.fn().mockImplementation(async () => sse(reply('ok')).body), cancel: vi.fn(), messages: vi.fn().mockRejectedValue(new Error('gone')), getRun: vi.fn().mockResolvedValue(runPB()) } as unknown as TryApi;
    let n = 0;
    const chat = useTryChat(api, () => `id-${++n}`, none);
    await chat.restore();
    expect(api.messages).not.toHaveBeenCalled();
    await chat.send('halo');
    expect(none.set).toHaveBeenLastCalledWith('id-1');
    chat.newChat();
    expect(none.set).toHaveBeenLastCalledWith('');
    await chat.send('baru');
    expect(none.set).toHaveBeenLastCalledWith('id-3');
    const gone = useTryChat(api, () => 'x', remembered('thread-gone'));
    await gone.restore();
    expect([gone.items.value.length, gone.error.value]).toEqual([0, null]);
  });
});

describe('try api', () => {
  const ok = (body: unknown = {}) => ({ ok: true, status: 200, body: 'stream', json: async () => body }) as unknown as Response;

  it('calls the console routes with the session cookie, and the run asks for an event stream', async () => {
    const fetcher = vi.fn().mockResolvedValue(ok({ messages: [], next_after: 0 }));
    const getRun = vi.fn();
    const api = tryApi('/base/console/', 'Ag 1', getRun, fetcher);
    const signal = new AbortController().signal;
    expect(await api.run({ threadId: 't', messages: [] }, signal)).toBe('stream');
    await api.cancel('R/1');
    await api.messages('t 1', 4);
    expect(fetcher.mock.calls.map(c => [c[0], c[1].method ?? 'GET', c[1].credentials])).toEqual([
      ['/base/console/agents/Ag%201/try/runs', 'POST', 'same-origin'],
      ['/base/console/agents/Ag%201/try/runs/R%2F1/cancel', 'POST', 'same-origin'],
      ['/base/console/agents/Ag%201/try/threads/t%201/messages?after=4&limit=200', 'GET', 'same-origin'],
    ]);
    expect(fetcher.mock.calls[0]![1]).toMatchObject({ headers: { 'Accept': 'text/event-stream', 'Content-Type': 'application/json' }, body: '{"threadId":"t","messages":[]}', signal });
    expect(api.getRun).toBe(getRun);
  });

  it('turns a refused call into an app error with its code and meta, or a bare one when the body is not JSON', async () => {
    const refusal = { ok: false, status: 400, statusText: 'Bad Request', json: async () => ({ code: 'AGT007', message: 'no model', request_id: 'req1', meta: { field: 'model' } }) } as unknown as Response;
    const api = tryApi('/console', 'a', vi.fn(), vi.fn().mockResolvedValue(refusal));
    const err = await api.cancel('r').catch(e => e);
    expect([errorCode(err), errorMeta(err)]).toEqual(['AGT007', { field: 'model' }]);
    const bare = await envelopeError({ statusText: 'Bad Gateway', json: async () => {
      throw new Error('html');
    } } as unknown as Response);
    expect([errorCode(bare), bare.rawMessage]).toEqual(['', 'Bad Gateway']);
  });
});
