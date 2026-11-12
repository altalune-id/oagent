import type { TurnRequest } from '../contract/index.ts';
import type { ToolResultReport } from '../turn/execute.ts';
import { Agent, FunctionTool } from '@strands-agents/sdk';
import { describe, expect, it } from 'vitest';

import { argsHash } from '../contract/index.ts';
import { ScriptedModel } from '../test/scripted-model.ts';
import { fakeMcp, goldenRequest, oldGoRequest, quiet } from '../test/turn-fixtures.ts';
import { executeTurn } from '../turn/execute.ts';
import { SDK_VERSION } from '../turn/snapshot.ts';
import { ApprovalCheck, MAX_GATED_PER_STEP, NO_SERVER, TOO_MANY } from './approval.ts';

const never = new AbortController().signal;
const THREAD = 't0193a2b4c5d67e8f9a0b1c2d3e4f5a6b';
const SCHEMA = { type: 'object', properties: { amount: { type: 'number' }, account: { type: 'string' }, confirm: { type: 'boolean' } } };
const ARGS = { confirm: true, amount: 25000, account: 'cash' };
const CONDITIONAL = { mode: 'conditional' as const, freeWhen: [{ arg: 'confirm', values: [false], absent: true }] };

function expense(calls: unknown[], schema: Record<string, unknown> = SCHEMA): FunctionTool {
  return new FunctionTool({
    name: 'yasaku__record_expense',
    description: 'Record an expense.',
    inputSchema: schema as never,
    callback: (input) => {
      calls.push(input);
      return { id: 'exp_1' };
    },
  });
}

function v3(over: (r: TurnRequest) => void = () => {}): TurnRequest {
  return goldenRequest('tool_call', (r) => {
    r.contractVersion = 3;
    const server = r.mcpServers[0] as unknown as Record<string, unknown>;
    delete server.allowedTools;
    server.tools = [{ name: 'record_expense', approval: CONDITIONAL }];
    over(r);
  });
}

async function pending(input: Record<string, unknown> = ARGS): Promise<{ request: TurnRequest, snapshot: unknown, id: string, hash: string }> {
  const calls: unknown[] = [];
  const out = await executeTurn(v3(), never, {
    model: new ScriptedModel([{ toolUse: { name: 'yasaku__record_expense', id: 'call_1', input } }]),
    log: quiet,
    openMcp: fakeMcp([expense(calls)]),
  });
  expect(out.status).toBe('awaiting_approval');
  expect(calls).toEqual([]);
  const [interrupt] = out.interrupts;
  const reason = interrupt!.reason as { argsHash: string };
  return { request: v3(), snapshot: out.snapshot, id: interrupt!.id, hash: reason.argsHash };
}

async function resume(response: unknown, steps = [{ text: ['ok'] }], over: (r: TurnRequest) => void = () => {}) {
  const p = await pending();
  const calls: unknown[] = [];
  const model = new ScriptedModel(steps);
  const out = await executeTurn(v3((r) => {
    r.snapshot = p.snapshot as never;
    r.input = { kind: 'resume', responses: [{ interruptId: p.id, response: typeof response === 'function' ? response(p.hash) : response }] };
    over(r);
  }), never, { model, log: quiet, openMcp: fakeMcp([expense(calls)]) });
  return { out, calls, model };
}

function toolResultText(model: ScriptedModel): string {
  const blocks = model.calls.flatMap(c => c.messages).flatMap(m => m.content);
  const result = blocks.find(b => b.type === 'toolResultBlock') as unknown as { content: { text?: string }[] };
  return result.content.map(c => c.text ?? '').join('');
}

describe('oagent:approval', () => {
  it('is named oagent:approval and denies when it throws', () => {
    const check = new ApprovalCheck(new Map());
    expect(check.name).toBe('oagent:approval');
    expect(check.onError).toBe('deny');
  });

  it('denies the call when the check itself throws (fails closed)', async () => {
    class Boom extends ApprovalCheck {
      override beforeToolCall(): never {
        throw new Error('boom');
      }
    }
    const calls: unknown[] = [];
    const agent = new Agent({
      model: new ScriptedModel([{ toolUse: { name: 'yasaku__record_expense', id: 'call_1', input: ARGS } }, { text: ['ok'] }]),
      tools: [expense(calls)],
      interventions: [new Boom(new Map())],
      sandbox: false,
      printer: false,
    });
    await agent.invoke('go');
    expect(calls).toEqual([]);
  });

  it('runs a call the rule frees without asking', async () => {
    const calls: unknown[] = [];
    const out = await executeTurn(v3(), never, {
      model: new ScriptedModel([{ toolUse: { name: 'yasaku__record_expense', id: 'call_1', input: { amount: 1, confirm: false } } }, { text: ['done'] }]),
      log: quiet,
      openMcp: fakeMcp([expense(calls)]),
    });
    expect(out.status).toBe('completed');
    expect(calls).toHaveLength(1);
  });

  it('interrupts a rule hit with the tool, the input, the args hash and the keys in schema order', async () => {
    const p = await pending();
    expect(p.id).toBe('hook:beforeToolCall:call_1:oagent:approval');
    const calls: unknown[] = [];
    const out = await executeTurn(v3(), never, { model: new ScriptedModel([{ toolUse: { name: 'yasaku__record_expense', id: 'call_1', input: ARGS } }]), log: quiet, openMcp: fakeMcp([expense(calls)]) });
    expect(out.interrupts).toEqual([{
      id: 'hook:beforeToolCall:call_1:oagent:approval',
      name: 'oagent:approval',
      reason: { toolUseId: 'call_1', server: 'yasaku', tool: 'record_expense', input: ARGS, argsHash: argsHash(ARGS), keys: ['amount', 'account', 'confirm'] },
    }]);
  });

  it('runs the approved call once after a resume in a fresh process', async () => {
    const { out, calls } = await resume((hash: string) => ({ approved: true, text: '', argsHash: hash }));
    expect(out.status).toBe('completed');
    expect(calls).toEqual([ARGS]);
  });

  it('denies a rejected call with the person\'s exact words and never runs it', async () => {
    const { out, calls, model } = await resume((hash: string) => ({ approved: false, text: 'no, it was 20000', argsHash: hash }));
    expect(out.status).toBe('completed');
    expect(calls).toEqual([]);
    expect(toolResultText(model)).toContain('no, it was 20000');
  });

  it('gives the model the deny text, not a stored result, when Go answers a denial for a call it also stored', async () => {
    const stored = 'STORED: exp_9 recorded before the restart';
    const { out, calls, model } = await resume(
      (hash: string) => ({ approved: false, text: 'outcome unknown after a restart', argsHash: hash }),
      [{ text: ['ok'] }],
      (r) => {
        r.storedResults = [{ toolUseId: 'call_1', result: { status: 'success', content: [{ text: stored }] } }];
      },
    );
    expect(out.status).toBe('completed');
    expect(calls).toEqual([]);
    expect(toolResultText(model)).toContain('outcome unknown after a restart');
    expect(toolResultText(model)).not.toContain(stored);
  });

  it('denies an approval whose hash does not match the call', async () => {
    const { calls, model } = await resume({ approved: true, text: '', argsHash: argsHash({ amount: 1 }) });
    expect(calls).toEqual([]);
    expect(toolResultText(model)).toContain('does not match');
  });

  it('denies a malformed answer', async () => {
    const { calls } = await resume({ approved: 'yes' });
    expect(calls).toEqual([]);
  });

  it('asks every call when the rule names an argument the live schema lacks', async () => {
    const calls: unknown[] = [];
    const out = await executeTurn(v3(), never, {
      model: new ScriptedModel([{ toolUse: { name: 'yasaku__record_expense', id: 'call_1', input: { amount: 1, confirm: false } } }]),
      log: quiet,
      openMcp: fakeMcp([expense(calls, { type: 'object', properties: { amount: { type: 'number' } } })]),
    });
    expect(out.status).toBe('awaiting_approval');
    expect(calls).toEqual([]);
  });

  it('lets an unknown tool name through with no interrupt, so the SDK\'s tool-not-found error reaches the model', async () => {
    const calls: unknown[] = [];
    const model = new ScriptedModel([{ toolUse: { name: 'nope__missing', id: 'c7', input: {} } }, { text: ['sorry'] }]);
    const out = await executeTurn(v3(), never, { model, log: quiet, openMcp: fakeMcp([expense(calls)]) });
    expect(out.status).toBe('completed');
    expect(out.interrupts).toEqual([]);
    expect(calls).toEqual([]);
  });

  it('denies a tool whose server is not in the request, with no interrupt (fails closed)', async () => {
    const calls: unknown[] = [];
    const other = new FunctionTool({ name: 'other__write', description: 'w', inputSchema: { type: 'object' }, callback: () => calls.push(1) });
    const model = new ScriptedModel([{ toolUse: { name: 'other__write', id: 'c9', input: {} } }, { text: ['ok'] }]);
    const out = await executeTurn(v3(), never, { model, log: quiet, openMcp: fakeMcp([other]) });
    expect(out.status).toBe('completed');
    expect(out.interrupts).toEqual([]);
    expect(calls).toEqual([]);
    expect(toolResultText(model)).toContain(NO_SERVER);
  });

  it(`denies gated calls past ${MAX_GATED_PER_STEP} in one step`, async () => {
    const calls: unknown[] = [];
    const results: ToolResultReport[] = [];
    const uses = Array.from({ length: MAX_GATED_PER_STEP + 1 }, (_, i) => ({ name: 'yasaku__record_expense', id: `c${i}`, input: { amount: i, confirm: true } }));
    const out = await executeTurn(v3(), never, {
      model: new ScriptedModel([{ toolUses: uses }]),
      log: quiet,
      openMcp: fakeMcp([expense(calls)]),
      onToolResult: async (r) => {
        results.push(r);
      },
    });
    expect(out.interrupts).toHaveLength(MAX_GATED_PER_STEP);
    expect(calls).toEqual([]);
    expect(results.map(r => r.toolUseId)).toEqual([`c${MAX_GATED_PER_STEP}`]);
    expect(JSON.stringify(results[0]!.result)).toContain(TOO_MANY);
  });

  it('runs old Go\'s v2 allowedTools body ungated during the rollout (frozen fixture)', async () => {
    const calls: unknown[] = [];
    const old = oldGoRequest('tool_call_v2');
    expect(old.contractVersion).toBe(2);
    expect(old.mcpServers[0]!.allowedTools).toEqual(['record_expense']);
    expect(old.mcpServers[0]!.tools).toBeUndefined();
    const out = await executeTurn(old, never, {
      model: new ScriptedModel([{ toolUse: { name: 'yasaku__record_expense', id: 'call_1', input: ARGS } }, { text: ['done'] }]),
      log: quiet,
      openMcp: fakeMcp([expense(calls)]),
    });
    expect(out.status).toBe('completed');
    expect(calls).toHaveLength(1);
  });

  it('keeps the snapshot tagged so a fresh agent can load it', async () => {
    const p = await pending();
    const agent = new Agent({ model: new ScriptedModel([]), sandbox: false, printer: false });
    agent.loadSnapshot(p.snapshot as never);
    expect((p.snapshot as { appData: unknown }).appData).toEqual({ thread: THREAD, agentVersion: 3, sdk: SDK_VERSION });
  });
});
