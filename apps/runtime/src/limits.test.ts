import { describe, expect, it } from 'vitest';

import { costMicros, DEFAULT_MAX_STEPS, emptyUsage, overBudget, stepLimit, UsageMeter } from './limits.ts';

const prices = { input: 2_500_000, output: 10_000_000, cacheRead: 1_250_000, cacheWrite: 0 };

describe('costMicros', () => {
  it('rounds every token class up on its own, then sums', () => {
    const usage = { ...emptyUsage(), inputTokens: 1, outputTokens: 1, cacheReadInputTokens: 1, cacheWriteInputTokens: 1 };
    expect(costMicros(usage, prices)).toBe(3 + 10 + 2 + 0);
  });

  it('prices a million tokens at the per-million price', () => {
    expect(costMicros({ ...emptyUsage(), inputTokens: 1_000_000, outputTokens: 1_000_000 }, prices)).toBe(12_500_000);
  });
});

describe('overBudget', () => {
  it('is never over without a budget', () => {
    expect(overBudget({ ...emptyUsage(), inputTokens: 10 ** 9 }, null)).toBe(false);
  });

  it('is over when the cost reaches the remaining amount', () => {
    const usage = { ...emptyUsage(), outputTokens: 100 };
    expect(overBudget(usage, { remainingMicros: 1001, prices })).toBe(false);
    expect(overBudget(usage, { remainingMicros: 1000, prices })).toBe(true);
    expect(overBudget(emptyUsage(), { remainingMicros: 0, prices })).toBe(true);
  });
});

describe('usageMeter', () => {
  it('sums metadata across model calls', () => {
    const meter = new UsageMeter();
    meter.startCall(400);
    meter.addMetadata({ inputTokens: 10, outputTokens: 2, cacheReadInputTokens: 5 });
    meter.startCall(400);
    meter.addMetadata({ inputTokens: 20, outputTokens: 3, cacheWriteInputTokens: 7 });
    meter.closeCutCall();
    expect(meter.usage).toEqual({ inputTokens: 30, outputTokens: 5, cacheReadInputTokens: 5, cacheWriteInputTokens: 7, estimated: false });
  });

  it('estimates every call that ended without metadata, not only the last', () => {
    const meter = new UsageMeter();
    for (let i = 0; i < 3; i++) {
      meter.startCall(400);
      meter.addOutput('abcd');
    }
    meter.closeCutCall();
    expect(meter.usage).toEqual({ inputTokens: 300, outputTokens: 3, cacheReadInputTokens: 0, cacheWriteInputTokens: 0, estimated: true });
  });

  it('estimates a failed call only when it had streamed output', () => {
    const quietCall = new UsageMeter();
    quietCall.startCall(400);
    quietCall.closeFailedCall();
    expect(quietCall.usage.estimated).toBe(false);
    const streamed = new UsageMeter();
    streamed.startCall(400);
    streamed.addOutput('abcd');
    streamed.closeFailedCall();
    expect(streamed.usage).toMatchObject({ inputTokens: 100, outputTokens: 1, estimated: true });
  });

  it('estimates a call cut before its metadata', () => {
    const meter = new UsageMeter();
    meter.startCall(401);
    meter.addOutput('Let me think');
    meter.closeCutCall();
    expect(meter.usage).toEqual({ inputTokens: 101, outputTokens: 3, cacheReadInputTokens: 0, cacheWriteInputTokens: 0, estimated: true });
  });
});

describe('turn limits', async () => {
  const { executeTurn } = await import('./agent.ts');
  const { LIMIT_REPLY } = await import('./limits.ts');
  const { ScriptedModel } = await import('./test/scripted-model.ts');
  const { fakeMcp, goldenRequest, quiet, recordTool } = await import('./test/turn-fixtures.ts');
  const never = new AbortController().signal;

  it('cancels at the deadline with error deadline and an estimated usage', async () => {
    const model = new ScriptedModel([{ text: ['Let me think'], hang: true }]);
    const out = await executeTurn(goldenRequest('cancel', (r) => {
      r.limits.deadlineMs = 100;
    }), never, { model, log: quiet, openMcp: fakeMcp([]) });
    expect(out.status).toBe('cancelled');
    expect(out.error).toEqual({ code: 'deadline' });
    expect(out.usage.estimated).toBe(true);
    expect(out.usage.outputTokens).toBe(3);
    expect(out.snapshot).not.toBeNull();
    expect(out.newMessages.map(m => m.role)).toEqual(['user']);
  });

  it('cancels without an error code when the client goes away', async () => {
    const controller = new AbortController();
    const model = new ScriptedModel([{ text: ['Let me'], hang: true }]);
    setTimeout(() => controller.abort(), 50);
    const out = await executeTurn(goldenRequest('cancel', (r) => {
      r.limits.deadlineMs = 10_000;
    }), controller.signal, { model, log: quiet, openMcp: fakeMcp([]) });
    expect(out.status).toBe('cancelled');
    expect(out.error).toBeUndefined();
  });

  it('starts the deadline before the MCP servers open', async () => {
    const model = new ScriptedModel([{ text: ['quick'] }]);
    const slowMcp = async () => {
      await new Promise(resolve => setTimeout(resolve, 200));
      return fakeMcp([])();
    };
    const out = await executeTurn(goldenRequest('cancel', (r) => {
      r.limits.deadlineMs = 100;
    }), never, { model, log: quiet, openMcp: slowMcp });
    expect(out.status).toBe('cancelled');
    expect(out.error).toEqual({ code: 'deadline' });
    expect(model.calls).toHaveLength(0);
  });

  it('counts a model call that fails mid-stream, estimated', async () => {
    const model = new ScriptedModel([{ text: ['Twelve chars'], failAfterText: new Error('stream reset') }]);
    const out = await executeTurn(goldenRequest('plain_reply'), never, { model, log: quiet, openMcp: fakeMcp([]) });
    expect(out.status).toBe('failed');
    expect(out.usage.estimated).toBe(true);
    expect(out.usage.outputTokens).toBe(3);
    expect(out.usage.inputTokens).toBeGreaterThan(0);
  });

  it('uses the runtime default step limit when maxSteps is 0', async () => {
    expect(DEFAULT_MAX_STEPS).toBe(20);
    expect(stepLimit(0)).toBe(DEFAULT_MAX_STEPS);
    expect(stepLimit(3)).toBe(3);
    const steps = Array.from({ length: DEFAULT_MAX_STEPS + 1 }, (_, i) => ({ toolUse: { name: 'yasaku__record_expense', id: `call_${i}`, input: {} } }));
    const model = new ScriptedModel(steps);
    const out = await executeTurn(goldenRequest('tool_call', (r) => {
      r.agent.maxSteps = 0;
    }), never, { model, log: quiet, openMcp: fakeMcp([recordTool([])]) });
    expect(out.status).toBe('limit_reached');
    expect(model.calls).toHaveLength(DEFAULT_MAX_STEPS);
  });

  it('turns maxSteps into limit_reached', async () => {
    const model = new ScriptedModel([
      { toolUse: { name: 'yasaku__record_expense', id: 'call_1', input: {} } },
      { text: ['never reached'] },
    ]);
    const out = await executeTurn(goldenRequest('limit_reached', (r) => {
      r.limits.budget = null;
    }), never, { model, log: quiet, openMcp: fakeMcp([recordTool([])]) });
    expect(out.status).toBe('limit_reached');
    expect(out.stopReason).toBe('limitTurns');
    expect(model.calls).toHaveLength(1);
  });

  it('refuses the model call over budget with the set reply and no DENIED message', async () => {
    const model = new ScriptedModel([
      { toolUse: { name: 'yasaku__record_expense', id: 'call_1', input: {} }, usage: { inputTokens: 0, outputTokens: 100 } },
      { text: ['never reached'] },
    ]);
    const out = await executeTurn(goldenRequest('limit_reached', (r) => {
      r.agent.maxSteps = 10;
      r.limits.budget = { remainingMicros: 1000, prices: { input: 2_500_000, output: 10_000_000, cacheRead: 1_250_000, cacheWrite: 0 } };
    }), never, { model, log: quiet, openMcp: fakeMcp([recordTool([])]) });
    expect(model.calls).toHaveLength(1);
    expect(out.status).toBe('limit_reached');
    expect(out.stopReason).toBe('budget');
    const last = out.newMessages.at(-1)!;
    expect(last.content.map(c => (c.type === 'textBlock' ? c.text : c.type))).toEqual([LIMIT_REPLY]);
    expect(JSON.stringify(out.newMessages)).not.toContain('DENIED');
    expect(JSON.stringify(out.snapshot)).not.toContain('DENIED');
  });
});
