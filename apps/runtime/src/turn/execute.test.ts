import type { Snapshot } from '@strands-agents/sdk';
import type { ToolResultReport } from './execute.ts';
import { Buffer } from 'node:buffer';
import { Agent, FunctionTool, Message, ModelThrottledError, TextBlock } from '@strands-agents/sdk';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { ModelProvider, TurnRefusedError } from '../contract/index.ts';
import { createModels } from '../models/index.ts';
import { apiError, fakeAnthropic } from '../test/anthropic-api.ts';
import { ScriptedModel } from '../test/scripted-model.ts';
import { AskFirst, fakeMcp, goldenRequest, quiet, recordTool } from '../test/turn-fixtures.ts';
import { executeTurn } from './execute.ts';
import { historyToMessages } from './history.ts';
import { checkSnapshot, SDK_VERSION } from './snapshot.ts';

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

  it('returns the reply when the sliding window trims a full snapshot at the end of the turn', async () => {
    const messages = Array.from({ length: 40 }, (_, i) => new Message({ role: i % 2 === 0 ? 'user' : 'assistant', content: [new TextBlock(`m${i}`)] }));
    const full = new Agent({ model: new ScriptedModel([]), messages, sandbox: false, printer: false }).takeSnapshot({ preset: 'session', appData });
    const model = new ScriptedModel([{ text: ['Good morning!'], usage: { inputTokens: 3600, outputTokens: 5 } }]);
    const out = await executeTurn(goldenRequest('plain_reply', (r) => {
      r.snapshot = full as never;
    }), never, { model, log: quiet, openMcp: fakeMcp([]) });
    expect(out.status).toBe('completed');
    expect(out.newMessages.map(m => m.role)).toEqual(['user', 'assistant']);
    expect(out.newMessages[1]!.content).toEqual([new TextBlock('Good morning!')]);
    expect((out.snapshot!.data.messages as unknown[]).length, 'the snapshot itself stays trimmed').toBeLessThanOrEqual(40);
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

const PNG_BYTES = [0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0, 0, 0, 13];

function imageRequest(): ReturnType<typeof goldenRequest> {
  return goldenRequest('plain_reply', (r) => {
    r.contractVersion = 2;
    r.input = {
      kind: 'message',
      message: {
        id: 'msg_user_1',
        role: 'user',
        content: [
          { type: 'text', text: 'my receipt' },
          { type: 'image', source: { type: 'data', value: Buffer.from(PNG_BYTES).toString('base64'), mimeType: 'image/png' } },
        ],
      },
    } as never;
  });
}

class StatusError extends Error {
  readonly status: number;

  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

describe('image input', () => {
  it('passes an image part to the model as an image block with the decoded bytes', async () => {
    const model = new ScriptedModel([{ text: ['Lunch, 25,000.'] }]);
    const out = await executeTurn(imageRequest(), never, { model, log: quiet, openMcp: fakeMcp([]) });
    expect(out.status).toBe('completed');
    const blocks = model.calls[0]!.messages.at(-1)!.content;
    expect(blocks.map(b => b.type)).toEqual(['textBlock', 'imageBlock']);
    const img = blocks[1] as unknown as { format: string, source: { bytes: Uint8Array } };
    expect(img.format).toBe('png');
    expect([...img.source.bytes]).toEqual(PNG_BYTES);
  });

  it('keeps no image bytes in the snapshot after the turn', async () => {
    const model = new ScriptedModel([{ text: ['Lunch, 25,000.'] }]);
    const out = await executeTurn(imageRequest(), never, { model, log: quiet, openMcp: fakeMcp([]) });
    const snap = JSON.stringify(out.snapshot);
    expect(snap).toContain('[image removed]');
    expect(snap).not.toContain(Buffer.from(PNG_BYTES).toString('base64'));
    expect(snap).not.toContain('imageBlock');
  });

  it('reports image_unsupported when the model refuses a turn that carried images', async () => {
    const model = new ScriptedModel([{ error: new StatusError(400, 'this model does not support image input') }]);
    const out = await executeTurn(imageRequest(), never, { model, log: quiet, openMcp: fakeMcp([]) });
    expect(out.status).toBe('failed');
    expect(out.error).toEqual({ code: 'image_unsupported' });
  });

  it('keeps model_error for a client error on a text-only turn and for a server error with images', async () => {
    const textOnly = await executeTurn(goldenRequest('plain_reply'), never, { model: new ScriptedModel([{ error: new StatusError(400, 'bad request') }]), log: quiet, openMcp: fakeMcp([]) });
    expect(textOnly.error).toEqual({ code: 'model_error' });
    const server = await executeTurn(imageRequest(), never, { model: new ScriptedModel([{ error: new StatusError(500, 'upstream') }]), log: quiet, openMcp: fakeMcp([]) });
    expect(server.error).toEqual({ code: 'model_error' });
    const throttled = await executeTurn(imageRequest(), never, { model: new ScriptedModel([{ error: new StatusError(429, 'slow down') }]), log: quiet, openMcp: fakeMcp([]) });
    expect(throttled.error).toEqual({ code: 'model_error' });
    for (const status of [401, 403, 404]) {
      const auth = await executeTurn(imageRequest(), never, { model: new ScriptedModel([{ error: new StatusError(status, 'denied') }]), log: quiet, openMcp: fakeMcp([]) });
      expect(auth.error, `status ${status}`).toEqual({ code: 'model_error' });
    }
  });

  it.each([400, 413, 415, 422])('maps a %i with images to image_unsupported', async (status) => {
    const out = await executeTurn(imageRequest(), never, { model: new ScriptedModel([{ error: new StatusError(status, 'no images') }]), log: quiet, openMcp: fakeMcp([]) });
    expect(out.error).toEqual({ code: 'image_unsupported' });
  });
});

describe('anthropic model errors', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  function anthropicImageRequest(): ReturnType<typeof goldenRequest> {
    const request = imageRequest();
    request.agent.model.provider = ModelProvider.ANTHROPIC;
    return request;
  }

  function anthropic(responses: (() => Response)[]): { model: ReturnType<ReturnType<typeof createModels>['model']>, seen: unknown[] } {
    const api = fakeAnthropic(responses);
    vi.stubGlobal('fetch', api.fetch);
    const model = createModels().model('p1', { provider: 'anthropic', baseUrl: 'https://api.anthropic.com', apiKey: 'sk-test', modelId: 'claude-sonnet-5-5' }, 0);
    return { model, seen: api.seen };
  }

  it('maps an Anthropic 400 on an image turn to image_unsupported', async () => {
    const { model } = anthropic([() => apiError(400, 'Image does not match the provided media type')]);
    const out = await executeTurn(anthropicImageRequest(), never, { model, log: quiet, openMcp: fakeMcp([]) });
    expect(out.error).toEqual({ code: 'image_unsupported' });
  });

  it('keeps an Anthropic 400 that is not about the image on an image turn a model_error', async () => {
    const { model } = anthropic([() => apiError(400, 'max_tokens: 200000 > 128000, which is the maximum allowed number of output tokens')]);
    const out = await executeTurn(anthropicImageRequest(), never, { model, log: quiet, openMcp: fakeMcp([]) });
    expect(out.error).toEqual({ code: 'model_error' });
  });

  it.each([413, 415])('maps an Anthropic %i on an image turn to image_unsupported whatever the message', async (status) => {
    const { model } = anthropic([() => apiError(status, 'request too large')]);
    const out = await executeTurn(anthropicImageRequest(), never, { model, log: quiet, openMcp: fakeMcp([]) });
    expect(out.error).toEqual({ code: 'image_unsupported' });
  });

  it('keeps an Anthropic 429 a model_error and does not retry it', async () => {
    const { model, seen } = anthropic([() => apiError(429, 'rate limited'), () => apiError(429, 'rate limited')]);
    const out = await executeTurn(anthropicImageRequest(), never, { model, log: quiet, openMcp: fakeMcp([]) });
    expect(out.status).toBe('failed');
    expect(out.error).toEqual({ code: 'model_error' });
    expect(seen).toHaveLength(1);
    const stream = model.stream([new Message({ role: 'user', content: [new TextBlock('hi')] })]);
    await expect(stream[Symbol.asyncIterator]().next()).rejects.toBeInstanceOf(ModelThrottledError);
  });
});
