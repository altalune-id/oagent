import type { Message } from '@strands-agents/sdk';
import { Message as Msg, TextBlock, ToolResultBlock, ToolUseBlock } from '@strands-agents/sdk';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { createModels } from '../models/index.ts';
import { captureLogger } from '../test/logger.ts';
import { ScriptedModel } from '../test/scripted-model.ts';
import { fakeMcp, goldenRequest, quiet, recordTool } from '../test/turn-fixtures.ts';
import { toAgUiMessages } from './agui.ts';
import { createRunTurn, HEARTBEAT_MS } from './index.ts';

afterEach(() => {
  vi.useRealTimers();
});

describe('heartbeat', () => {
  it('emits a heartbeat every 15 s while the turn runs, and stops after', async () => {
    vi.useFakeTimers();
    const events: Record<string, unknown>[] = [];
    const runTurn = createRunTurn({
      models: createModels(),
      log: quiet,
      modelFor: () => new ScriptedModel([{ text: ['slow'], hang: true }]),
      openMcp: fakeMcp([]),
    });
    const controller = new AbortController();
    const request = goldenRequest('cancel', (r) => {
      r.limits.deadlineMs = 600_000;
    });
    const done = runTurn(request, async (e) => {
      events.push(e);
    }, controller.signal);
    await vi.advanceTimersByTimeAsync(HEARTBEAT_MS * 2 + 1);
    expect(events.filter(e => e.type === 'OAGENT_HEARTBEAT')).toHaveLength(2);
    controller.abort();
    await done;
    const count = events.filter(e => e.type === 'OAGENT_HEARTBEAT').length;
    await vi.advanceTimersByTimeAsync(HEARTBEAT_MS * 3);
    expect(events.filter(e => e.type === 'OAGENT_HEARTBEAT')).toHaveLength(count);
    expect(events.at(-1)!.type).toBe('OAGENT_TERMINAL');
  });
});

describe('turn outcome log', () => {
  it('logs the stop reason, tokens and the kinds of parts the model returned, never the text', async () => {
    const lines: string[] = [];
    const runTurn = createRunTurn({
      models: createModels(),
      log: captureLogger(lines),
      modelFor: () => new ScriptedModel([
        { toolUse: { name: 'yasaku__record_expense', id: 'call_1', input: { amount: 25000 } }, usage: { inputTokens: 900, outputTokens: 12 } },
        { text: ['Good morning!'], usage: { inputTokens: 950, outputTokens: 5 } },
      ]),
      openMcp: fakeMcp([recordTool([])]),
    });
    const request = goldenRequest('tool_call');
    await runTurn(request, async () => {}, new AbortController().signal);
    const outcome = lines.map(l => JSON.parse(l) as Record<string, unknown>).find(l => l.msg === 'turn outcome');
    expect(outcome).toMatchObject({
      level: 'info',
      run: request.ids.run,
      status: 'completed',
      stopReason: 'endTurn',
      inputTokens: 1850,
      outputTokens: 17,
      parts: ['toolUseBlock', 'textBlock'],
    });
    expect(lines.join('\n')).not.toContain('Good morning!');
    expect(lines.join('\n')).not.toContain('25000');
  });

  it('logs an empty answer as an empty part list', async () => {
    const lines: string[] = [];
    const runTurn = createRunTurn({
      models: createModels(),
      log: captureLogger(lines),
      modelFor: () => new ScriptedModel([{ text: ['  '] }]),
      openMcp: fakeMcp([]),
    });
    await runTurn(goldenRequest('plain_reply'), async () => {}, new AbortController().signal);
    const outcome = lines.map(l => JSON.parse(l) as Record<string, unknown>).find(l => l.msg === 'turn outcome');
    expect(outcome).toMatchObject({ status: 'completed', stopReason: 'endTurn', parts: ['empty'] });
  });
});

describe('terminal', () => {
  it('is the last line even when the turn throws outside the model', async () => {
    const events: Record<string, unknown>[] = [];
    const runTurn = createRunTurn({
      models: createModels(),
      log: quiet,
      modelFor: () => new ScriptedModel([]),
      openMcp: async () => {
        throw new Error('mcp exploded');
      },
    });
    await runTurn(goldenRequest('plain_reply'), async (e) => {
      events.push(e);
    }, new AbortController().signal);
    expect(events.map(e => e.type)).toEqual(['RUN_STARTED', 'RUN_ERROR', 'OAGENT_TERMINAL']);
    expect(events[1]).toEqual({ type: 'RUN_ERROR', message: 'runtime error', code: 'runtime_error' });
    expect(events[2]).toMatchObject({ status: 'failed', stopReason: 'error', snapshot: null, messagesAppended: [], error: { code: 'runtime_error' } });
  });

  it('streams the set reply when the budget refuses the model call', async () => {
    const events: Record<string, unknown>[] = [];
    const runTurn = createRunTurn({
      models: createModels(),
      log: quiet,
      modelFor: () => new ScriptedModel([]),
      openMcp: fakeMcp([]),
    });
    const request = goldenRequest('limit_reached', (r) => {
      r.limits.budget = { remainingMicros: 0, prices: { input: 1, output: 1, cacheRead: 1, cacheWrite: 1 } };
    });
    await runTurn(request, async (e) => {
      events.push(e);
    }, new AbortController().signal);
    expect(events.map(e => e.type)).toEqual(['RUN_STARTED', 'TEXT_MESSAGE_START', 'TEXT_MESSAGE_CONTENT', 'TEXT_MESSAGE_END', 'RUN_FINISHED', 'OAGENT_TERMINAL']);
    const terminal = events.at(-1)!;
    expect(terminal).toMatchObject({ status: 'limit_reached', stopReason: 'budget' });
    expect((terminal.messagesAppended as { id: string }[]).map(m => m.id)).toEqual(['msg_user_1', 'msg_asst_1']);
    expect(events[1]).toMatchObject({ messageId: 'msg_asst_1' });
  });

  it('refuses an unloadable snapshot before any event', async () => {
    const events: unknown[] = [];
    const runTurn = createRunTurn({ models: createModels(), log: quiet, modelFor: () => new ScriptedModel([]), openMcp: fakeMcp([]) });
    const request = goldenRequest('resume', (r) => {
      r.snapshot = { ...r.snapshot, appData: { thread: r.ids.thread, agentVersion: 99, sdk: '1.23.0' } };
    });
    await expect(runTurn(request, async (e) => {
      events.push(e);
    }, new AbortController().signal)).rejects.toMatchObject({ status: 422, code: 'snapshot_unloadable' });
    expect(events).toEqual([]);
  });

  it('refuses invalid image input before any event', async () => {
    const events: unknown[] = [];
    const runTurn = createRunTurn({ models: createModels(), log: quiet, modelFor: () => new ScriptedModel([]), openMcp: fakeMcp([]) });
    const request = goldenRequest('plain_reply', (r) => {
      r.input = { kind: 'message', message: { id: 'u1', role: 'user', content: [{ type: 'image', source: { type: 'url', value: 'https://example.com/a.png' } }] } } as never;
    });
    await expect(runTurn(request, async (e) => {
      events.push(e);
    }, new AbortController().signal)).rejects.toMatchObject({ status: 422, code: 'image_invalid' });
    expect(events).toEqual([]);
  });

  it('builds the model from the cached clients by default', async () => {
    const models = createModels();
    const spy = vi.spyOn(models, 'model').mockReturnValue(new ScriptedModel([{ text: ['ok'] }]) as never);
    const runTurn = createRunTurn({ models, log: quiet, openMcp: fakeMcp([]) });
    await runTurn(goldenRequest('plain_reply'), async () => {}, new AbortController().signal);
    expect(spy).toHaveBeenCalledWith('Pj4kW9sQ2mN7vB3xC8zLr', expect.objectContaining({ modelId: 'gpt-4o' }), 0.2);
  });
});

describe('toAgUiMessages', () => {
  it('maps user, assistant text and tool calls, and tool results', () => {
    const request = goldenRequest('tool_call');
    const messages: Message[] = [
      new Msg({ role: 'user', content: [new TextBlock('record lunch 25000 from cash')] }),
      new Msg({ role: 'assistant', content: [new TextBlock('On it.'), new ToolUseBlock({ name: 'x', toolUseId: 'c1', input: { a: 1 } })] }),
      new Msg({ role: 'user', content: [
        new ToolResultBlock({ toolUseId: 'c1', status: 'success', content: [new TextBlock('done')] }),
      ] }),
      new Msg({ role: 'assistant', content: [new TextBlock('All set.')] }),
    ];
    expect(toAgUiMessages(request, messages)).toEqual([
      { id: 'msg_user_1', role: 'user', content: 'record lunch 25000 from cash' },
      { id: 'msg_asst_1', role: 'assistant', content: 'On it.', toolCalls: [{ id: 'c1', type: 'function', function: { name: 'x', arguments: '{"a":1}' } }] },
      { id: 'msg_tool_c1', role: 'tool', toolCallId: 'c1', content: 'done' },
      { id: 'msg_asst_2', role: 'assistant', content: 'All set.' },
    ]);
  });

  it('echoes the user message without image bytes: Go already holds the input, and the bytes would double the stream', () => {
    const request = goldenRequest('plain_reply', (r) => {
      r.input = { kind: 'message', message: { id: 'msg_user_1', role: 'user', content: [
        { type: 'text', text: 'my receipt' },
        { type: 'image', source: { type: 'data', value: 'iVBORw0KGgo=', mimeType: 'image/png' } },
      ] } } as never;
    });
    const out = toAgUiMessages(request, [new Msg({ role: 'user', content: [new TextBlock('my receipt')] })]);
    expect(out).toEqual([{ id: 'msg_user_1', role: 'user', content: [{ type: 'text', text: 'my receipt' }, { type: 'text', text: '[image removed]' }] }]);
    expect(JSON.stringify(out)).not.toContain('iVBORw0KGgo=');
  });
});
