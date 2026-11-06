import type { Snapshot } from '@strands-agents/sdk';
import type { ToolResultReport } from './agent.ts';
import { Agent, FunctionTool, ModelThrottledError } from '@strands-agents/sdk';
import { describe, expect, it } from 'vitest';

import { TurnRefusedError } from '../http/contract.ts';
import { ScriptedModel } from '../test/scripted-model.ts';
import { AskFirst, fakeMcp, goldenRequest, quiet, recordTool } from '../test/turn-fixtures.ts';
import { checkSnapshot, executeTurn, historyToMessages } from './agent.ts';
import { SDK_VERSION } from './snapshot.ts';

const THREAD = 't0193a2b4c5d67e8f9a0b1c2d3e4f5a6b';
const appData = { thread: THREAD, agentVersion: 3, sdk: SDK_VERSION };
const never = new AbortController().signal;

async function priorSnapshot(): Promise<Snapshot> {
  const agent = new Agent({ model: new ScriptedModel([{ text: ['Hi there.'] }]), systemPrompt: 'OLD instructions', sandbox: false, printer: false });
  await agent.invoke('hello');
  return agent.takeSnapshot({ preset: 'session', appData });
}

async function pendingSnapshot(): Promise<Snapshot> {
  const agent = new Agent({
    model: new ScriptedModel([{ toolUse: { name: 'yasaku__record_expense', id: 'call_1', input: { amount: 25000 } } }]),
    tools: [recordTool([])],
    interventions: [new AskFirst()],
    sandbox: false,
    printer: false,
  });
  const result = await agent.invoke('record lunch');
  expect(result.interrupts?.map(i => i.id)).toEqual(['hook:beforeToolCall:call_1:test:approval']);
  return agent.takeSnapshot({ preset: 'session', appData });
}

describe('executeTurn', () => {
  it('loads the snapshot before the run and lets the request instructions win', async () => {
    const snapshot = await priorSnapshot();
    const model = new ScriptedModel([{ text: ['Done.'], usage: { inputTokens: 30, outputTokens: 2 } }]);
    const request = goldenRequest('plain_reply', (r) => {
      r.snapshot = snapshot as never;
      r.agent.instructions = 'NEW instructions';
    });
    const out = await executeTurn(request, never, { model, log: quiet, openMcp: fakeMcp([]) });
    expect(out.status).toBe('completed');
    expect(model.calls[0]!.systemPrompt).toBe('NEW instructions');
    const texts = model.calls[0]!.messages.flatMap(m => m.content.map(c => (c.type === 'textBlock' ? c.text : c.type)));
    expect(texts.slice(0, 2)).toEqual(['hello', 'Hi there.']);
    expect(out.usage).toEqual({ inputTokens: 30, outputTokens: 2, cacheReadInputTokens: 0, cacheWriteInputTokens: 0, estimated: false });
  });

  it('takes a session snapshot tagged with the thread, agent version and SDK version', async () => {
    const model = new ScriptedModel([{ text: ['Hello!'] }]);
    const out = await executeTurn(goldenRequest('plain_reply'), never, { model, log: quiet, openMcp: fakeMcp([]) });
    expect(out.snapshot?.appData).toEqual(appData);
    expect(Object.keys(out.snapshot!.data).sort()).toEqual(['interrupts', 'messages', 'modelState', 'state', 'systemPrompt']);
    expect(out.newMessages.map(m => m.role)).toEqual(['user', 'assistant']);
    expect(model.calls[0]!.messages.map(m => m.role), 'no snapshot and no history: a fresh agent sees only the new message').toEqual(['user']);
  });

  it('seeds the agent from history when there is no snapshot', async () => {
    const model = new ScriptedModel([{ text: ['Yes.'] }]);
    const request = goldenRequest('rebuild_from_history');
    await executeTurn(request, never, { model, log: quiet, openMcp: fakeMcp([recordTool([])]) });
    const kinds = model.calls[0]!.messages.map(m => `${m.role}:${m.content.map(c => c.type).join('+')}`);
    expect(kinds).toEqual([
      'user:textBlock',
      'assistant:toolUseBlock',
      'user:toolResultBlock',
      'assistant:textBlock',
      'user:textBlock',
    ]);
  });

  it('returns a stored result instead of calling the tool, marked replayed', async () => {
    const calls: unknown[] = [];
    const checkpoints: ToolResultReport[] = [];
    const snapshot = await pendingSnapshot();
    const model = new ScriptedModel([{ text: ['Recorded.'] }]);
    const request = goldenRequest('stored_result', (r) => {
      r.snapshot = snapshot as never;
      r.input = { kind: 'resume', responses: [{ interruptId: 'hook:beforeToolCall:call_1:test:approval', response: { approved: true } }] };
    });
    const out = await executeTurn(request, never, { model, log: quiet, openMcp: fakeMcp([recordTool(calls)]), onToolResult: c => void checkpoints.push(c) });
    expect(calls).toEqual([]);
    expect(checkpoints.map(c => [c.toolUseId, c.replayed])).toEqual([['call_1', true]]);
    expect(out.status).toBe('completed');
    const toolResult = model.calls[0]!.messages.at(-1)!.content[0]!;
    expect(toolResult.type).toBe('toolResultBlock');
    expect(JSON.stringify(toolResult)).toContain('exp_1');
  });

  it('checkpoints a completed tool call', async () => {
    const calls: unknown[] = [];
    const checkpoints: ToolResultReport[] = [];
    const model = new ScriptedModel([
      { toolUse: { name: 'yasaku__record_expense', id: 'call_1', input: { amount: 25000 } } },
      { text: ['Recorded.'] },
    ]);
    const out = await executeTurn(goldenRequest('tool_call'), never, { model, log: quiet, openMcp: fakeMcp([recordTool(calls)]), onToolResult: c => void checkpoints.push(c) });
    expect(calls).toEqual([{ amount: 25000 }]);
    expect(checkpoints).toEqual([{ toolUseId: 'call_1', name: 'yasaku__record_expense', input: { amount: 25000 }, result: { status: 'success', content: [{ json: { id: 'exp_1' } }] }, replayed: false }]);
    expect(out.newMessages.map(m => m.role)).toEqual(['user', 'assistant', 'user', 'assistant']);
  });

  it('stops on a tool interrupt with awaiting_approval and lists it', async () => {
    const ask = new FunctionTool({
      name: 'yasaku__record_expense',
      description: 'Asks first.',
      inputSchema: { type: 'object' },
      callback: (_input, ctx) => ctx.interrupt<{ ok: boolean }>({ name: 'lookup', reason: { q: 'x' } }).ok,
    });
    const model = new ScriptedModel([{ toolUse: { name: 'yasaku__record_expense', id: 'call_1', input: {} } }]);
    const out = await executeTurn(goldenRequest('tool_call'), never, { model, log: quiet, openMcp: fakeMcp([ask]) });
    expect(out.status).toBe('awaiting_approval');
    expect(out.interrupts).toEqual([{ id: 'tool:call_1:lookup', name: 'lookup', reason: { q: 'x' } }]);
    expect(out.snapshot).not.toBeNull();
  });

  it('fails without a snapshot and keeps the completed tool round', async () => {
    const model = new ScriptedModel([
      { toolUse: { name: 'yasaku__record_expense', id: 'call_1', input: {} } },
      { error: new Error('upstream 500 sk-test') },
    ]);
    const out = await executeTurn(goldenRequest('model_error'), never, { model, log: quiet, openMcp: fakeMcp([recordTool([])]) });
    expect(out.status).toBe('failed');
    expect(out.snapshot).toBeNull();
    expect(out.error).toEqual({ code: 'model_error' });
    expect(out.newMessages.map(m => m.role)).toEqual(['user', 'assistant', 'user']);
  });

  it('adds a down server note and prompt line', async () => {
    const model = new ScriptedModel([{ text: ['Sorry.'] }]);
    const out = await executeTurn(goldenRequest('mcp_server_down'), never, {
      model,
      log: quiet,
      openMcp: fakeMcp([], { notes: [{ kind: 'mcp_server_unavailable', server: 'yasaku' }], promptLines: ['Tools from yasaku are unavailable in this turn.'] }),
    });
    expect(out.notes).toEqual([{ kind: 'mcp_server_unavailable', server: 'yasaku' }]);
    expect(String(model.calls[0]!.systemPrompt)).toContain('\n\nTools from yasaku are unavailable in this turn.');
  });
});

describe('continue', () => {
  it('seeds the completed tool round and lets the model go on with no new user message', async () => {
    const calls: unknown[] = [];
    const model = new ScriptedModel([{ text: ['Recorded.'] }]);
    const out = await executeTurn(goldenRequest('resume_after_crash'), never, { model, log: quiet, openMcp: fakeMcp([recordTool(calls)]) });
    expect(out.status).toBe('completed');
    expect(calls).toEqual([]);
    expect(model.calls).toHaveLength(1);
    const seen = model.calls[0]!.messages.map(m => `${m.role}:${m.content.map(c => c.type).join('+')}`);
    expect(seen).toEqual(['user:textBlock', 'assistant:toolUseBlock', 'user:toolResultBlock']);
    expect(out.newMessages.map(m => m.role)).toEqual(['assistant']);
  });
});

describe('model retries', () => {
  it('leaves retries to Go: a throttled model call is not retried', async () => {
    const model = new ScriptedModel([{ error: new ModelThrottledError('slow down') }, { text: ['retried'] }]);
    const out = await executeTurn(goldenRequest('plain_reply'), never, { model, log: quiet, openMcp: fakeMcp([]) });
    expect(model.calls).toHaveLength(1);
    expect(out.status).toBe('failed');
  });
});

describe('refusal text', () => {
  it('keeps a real model reply that starts with DENIED:', async () => {
    const model = new ScriptedModel([{ text: ['DENIED: your card was declined'] }]);
    const out = await executeTurn(goldenRequest('plain_reply'), never, { model, log: quiet, openMcp: fakeMcp([]) });
    expect(out.newMessages.map(m => m.role)).toEqual(['user', 'assistant']);
    expect(JSON.stringify(out.snapshot)).toContain('your card was declined');
  });
});

describe('checkSnapshot', () => {
  it('accepts no snapshot and a matching one', async () => {
    expect(() => checkSnapshot(goldenRequest('plain_reply'))).not.toThrow();
    const snapshot = await priorSnapshot();
    expect(() => checkSnapshot(goldenRequest('plain_reply', (r) => {
      r.snapshot = snapshot as never;
    }))).not.toThrow();
  });

  it('refuses a snapshot with other tags or one that does not load', async () => {
    const snapshot = await priorSnapshot();
    const wrongTags = goldenRequest('plain_reply', (r) => {
      r.snapshot = { ...snapshot, appData: { ...appData, agentVersion: 2 } } as never;
    });
    expect(() => checkSnapshot(wrongTags)).toThrow(TurnRefusedError);
    const broken = goldenRequest('plain_reply', (r) => {
      r.snapshot = { ...snapshot, schemaVersion: '9.0' } as never;
    });
    try {
      checkSnapshot(broken);
      expect.unreachable();
    } catch (error) {
      expect(error).toMatchObject({ status: 422, code: 'snapshot_unloadable' });
    }
  });
});

describe('historyToMessages', () => {
  it('merges consecutive tool results and reads text parts', () => {
    const out = historyToMessages([
      { id: 'u', role: 'user', content: [{ type: 'text', text: 'a' }, { type: 'text', text: 'b' }] } as never,
      { id: 'a', role: 'assistant', toolCalls: [{ id: 'c1', type: 'function', function: { name: 'x', arguments: '' } }, { id: 'c2', type: 'function', function: { name: 'y', arguments: '{"k":1}' } }] },
      { id: 't1', role: 'tool', toolCallId: 'c1', content: 'r1' },
      { id: 't2', role: 'tool', toolCallId: 'c2', content: 'r2', error: 'bad' } as never,
      { id: 's', role: 'system', content: 'ignored' },
    ]);
    expect(out).toEqual([
      { role: 'user', content: [{ text: 'ab' }] },
      { role: 'assistant', content: [{ toolUse: { name: 'x', toolUseId: 'c1', input: {} } }, { toolUse: { name: 'y', toolUseId: 'c2', input: { k: 1 } } }] },
      { role: 'user', content: [
        { toolResult: { toolUseId: 'c1', status: 'success', content: [{ text: 'r1' }] } },
        { toolResult: { toolUseId: 'c2', status: 'error', content: [{ text: 'r2' }] } },
      ] },
    ]);
  });
});
