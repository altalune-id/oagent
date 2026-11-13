import type { Model, ModelStreamEvent } from '@strands-agents/sdk';
import type { TurnRequest } from '../contract/index.ts';
import type { Models } from '../models/index.ts';
import type { Step } from '../test/scripted-model.ts';
import { Agent, FunctionTool, Message, TextBlock } from '@strands-agents/sdk';
import { describe, expect, it, vi } from 'vitest';

import { ModelProvider } from '../contract/index.ts';
import { ScriptedModel } from '../test/scripted-model.ts';
import { fakeMcp, goldenRequest, quiet } from '../test/turn-fixtures.ts';
import { executeTurn } from './execute.ts';
import { HelperBudgetError, helperModel } from './helper.ts';
import { emptyUsage } from './limits.ts';

const helperSpec = { provider: ModelProvider.OPENAI_COMPATIBLE, baseUrl: 'https://helper.example.com/v1', apiKey: 'sk-helper', modelId: 'small' };
const prices = { input: 2_500_000, output: 10_000_000, cacheRead: 1_250_000, cacheWrite: 0 };
const never = new AbortController().signal;

function stubModels(model: Model): Models & { model: ReturnType<typeof vi.fn> } {
  return { model: vi.fn(() => model) };
}

function ask(text = 'extract facts'): Message[] {
  return [new Message({ role: 'user', content: [new TextBlock(text)] })];
}

async function drain(events: AsyncIterable<ModelStreamEvent>): Promise<void> {
  const iterator = events[Symbol.asyncIterator]();
  let next = await iterator.next();
  while (!next.done) {
    next = await iterator.next();
  }
}

function setup(steps: Step[], over: (r: TurnRequest) => void = () => {}) {
  const scripted = new ScriptedModel(steps);
  const models = stubModels(scripted);
  const request = goldenRequest('plain_reply', over);
  const usage = emptyUsage();
  return { scripted, models, request, usage, helper: helperModel(request, models, usage) };
}

describe('helperModel', () => {
  it('uses the request helper model when set', () => {
    const { models, request } = setup([], (r) => {
      r.agent.helperModel = helperSpec;
    });
    expect(models.model).toHaveBeenCalledWith(request.ids.project, helperSpec, request.agent.temperature);
  });

  it('falls back to the main model spec', () => {
    const { models, request } = setup([]);
    expect(models.model).toHaveBeenCalledWith(request.ids.project, request.agent.model, request.agent.temperature);
  });

  it('meters a call into usage.helper and leaves the main classes untouched', async () => {
    const { helper, usage } = setup([{ text: ['ok'], usage: { inputTokens: 120, outputTokens: 9, cacheReadInputTokens: 80 } }]);
    await drain(helper.stream(ask()));
    expect(usage).toEqual({ ...emptyUsage(), helper: { inputTokens: 120, outputTokens: 9, cacheReadInputTokens: 80, cacheWriteInputTokens: 0 } });
  });

  it('meters a call through streamAggregated as through stream', async () => {
    const { helper, usage } = setup([{ text: ['ok'], usage: { inputTokens: 30, outputTokens: 4 } }]);
    await new Agent({ model: helper, sandbox: false, printer: false }).invoke('extract facts');
    expect(usage.helper).toEqual({ inputTokens: 30, outputTokens: 4, cacheReadInputTokens: 0, cacheWriteInputTokens: 0 });
  });

  it('estimates a helper call cut before its metadata', async () => {
    const { helper, usage } = setup([{ text: ['abcd', 'efgh'], usage: { inputTokens: 1, outputTokens: 1 } }]);
    for await (const e of helper.stream(ask())) {
      if (e.type === 'modelContentBlockDeltaEvent') {
        break;
      }
    }
    expect(usage.estimated).toBe(true);
    expect(usage.helper?.outputTokens).toBe(1);
    expect(usage.helper?.inputTokens).toBeGreaterThan(0);
    expect(usage.inputTokens + usage.outputTokens).toBe(0);
  });

  it('counts nothing for a call that failed before any output', async () => {
    const boom = new Error('upstream 500');
    const { helper, usage } = setup([{ error: boom }]);
    await expect(drain(helper.stream(ask()))).rejects.toBe(boom);
    expect(Object.keys(usage)).not.toContain('helper');
    expect(usage.estimated).toBe(false);
  });

  it('meters two interleaved helper calls without mixing them', async () => {
    const { helper, usage } = setup([
      { text: ['a', 'b'], usage: { inputTokens: 10, outputTokens: 2 } },
      { text: ['c'], usage: { inputTokens: 20, outputTokens: 3, cacheWriteInputTokens: 5 } },
    ]);
    const first = helper.stream(ask('one'))[Symbol.asyncIterator]();
    const second = helper.stream(ask('two'))[Symbol.asyncIterator]();
    let firstDone = false;
    let secondDone = false;
    while (!firstDone || !secondDone) {
      if (!firstDone) {
        firstDone = (await first.next()).done ?? false;
      }
      if (!secondDone) {
        secondDone = (await second.next()).done ?? false;
      }
    }
    expect(usage).toEqual({ ...emptyUsage(), helper: { inputTokens: 30, outputTokens: 5, cacheReadInputTokens: 0, cacheWriteInputTokens: 5 } });
  });

  it('refuses a call when the turn is already over budget', async () => {
    const { helper, usage, scripted } = setup([{ text: ['never'] }], (r) => {
      r.agent.helperModel = helperSpec;
      r.limits.budget = { remainingMicros: 1000, prices, helperPrices: prices };
    });
    usage.outputTokens = 100;
    await expect(drain(helper.stream(ask()))).rejects.toBeInstanceOf(HelperBudgetError);
    expect(scripted.calls).toHaveLength(0);
  });

  it('runs a call under the budget', async () => {
    const { helper, usage } = setup([{ text: ['ok'], usage: { inputTokens: 10, outputTokens: 1 } }], (r) => {
      r.limits.budget = { remainingMicros: 1001, prices };
    });
    usage.outputTokens = 100;
    await drain(helper.stream(ask()));
    expect(usage.helper?.inputTokens).toBe(10);
  });

  it('passes config and model info through to the wrapped model', () => {
    const { helper, scripted } = setup([]);
    helper.updateConfig({ modelId: 'renamed' });
    expect(scripted.getConfig().modelId).toBe('renamed');
    expect(helper.getConfig()).toBe(scripted.getConfig());
    expect(helper.modelId).toBe('renamed');
    expect(helper.modelInfo).toEqual(scripted.modelInfo);
  });

  it('never feeds the main meter', async () => {
    const helperUsage = emptyUsage();
    const helperScript = new ScriptedModel([{ text: ['fact'], usage: { inputTokens: 50, outputTokens: 5 } }]);
    const request = goldenRequest('tool_call');
    const helper = helperModel(request, stubModels(helperScript), helperUsage);
    const tool = new FunctionTool({
      name: 'yasaku__record_expense',
      description: 'Record an expense.',
      inputSchema: { type: 'object' } as never,
      callback: async () => {
        await drain(helper.stream(ask()));
        return { id: 'exp_1' };
      },
    });
    const main = new ScriptedModel([
      { toolUse: { name: 'yasaku__record_expense', id: 'call_1', input: {} }, usage: { inputTokens: 450, outputTokens: 40 } },
      { text: ['Recorded.'], usage: { inputTokens: 460, outputTokens: 24 } },
    ]);
    const out = await executeTurn(request, never, { model: main, log: quiet, openMcp: fakeMcp([tool]) });
    expect(out.usage).toEqual({ inputTokens: 910, outputTokens: 64, cacheReadInputTokens: 0, cacheWriteInputTokens: 0, estimated: false });
    expect(helperUsage.helper).toEqual({ inputTokens: 50, outputTokens: 5, cacheReadInputTokens: 0, cacheWriteInputTokens: 0 });
  });
});
