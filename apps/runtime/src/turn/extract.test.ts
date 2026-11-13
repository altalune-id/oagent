import type { Model } from '@strands-agents/sdk';
import type { TurnRequest } from '../contract/index.ts';
import type { Models } from '../models/index.ts';
import type { Step } from '../test/scripted-model.ts';
import type { TurnOutcome } from './execute.ts';
import { FunctionTool } from '@strands-agents/sdk';
import { describe, expect, it, vi } from 'vitest';

import { MemoryExtract } from '../contract/index.ts';
import { maskText } from '../guardrails/mask.ts';
import { captureLogger } from '../test/logger.ts';
import { ScriptedModel } from '../test/scripted-model.ts';
import { fakeMcp, goldenRequest, quiet, recordTool } from '../test/turn-fixtures.ts';
import { escapeToolOutput } from './delimit.ts';
import { executeTurn } from './execute.ts';
import { EXTRACT_MARGIN_MS, EXTRACT_MAX_CALLS, EXTRACT_MAX_MS, EXTRACT_MIN_MS, extractWindowMs, MAX_NEW_FACTS } from './extract.ts';
import { createRunTurn } from './index.ts';
import { EXTRACT_OUTPUT_TOKENS } from './limits.ts';

const never = new AbortController().signal;
const ALL = { email: true, phone: true, card: true, secrets: true, keyPrefix: 'oagent_' };
const FACTS = [{ text: 'Prefers WhatsApp after 6pm.', createdAt: '2026-11-01T09:30:00.123456789Z' }];
const PASSAGES = [{ text: 'Refunds take 7 days.', title: 'Refund policy' }];
const prices = { input: 2_500_000, output: 10_000_000, cacheRead: 1_250_000, cacheWrite: 0 };

function factsStep(facts: unknown, usage = { inputTokens: 40, outputTokens: 6 }): Step {
  return { toolUse: { name: 'strands_structured_output', id: 'so_1', input: { facts } as Record<string, unknown> }, usage };
}

function stubModels(model: Model): Models & { model: ReturnType<typeof vi.fn> } {
  return { model: vi.fn(() => model) };
}

type Run = { out: TurnOutcome, helper: ScriptedModel, main: ScriptedModel, lines: string[] };

async function run(over: (r: TurnRequest) => void, helperSteps: Step[], mainSteps: Step[] = [{ text: ['Noted.'], usage: { inputTokens: 100, outputTokens: 3 } }], golden = 'plain_reply'): Promise<Run> {
  const helper = new ScriptedModel(helperSteps);
  const main = new ScriptedModel(mainSteps);
  const lines: string[] = [];
  const request = goldenRequest(golden, (r) => {
    r.memory = { facts: [], extract: MemoryExtract.FACTS };
    r.guardrails = { keyPrefix: 'oagent_' };
    over(r);
  });
  const out = await executeTurn(request, never, { model: main, models: stubModels(helper), log: captureLogger(lines), openMcp: fakeMcp([recordTool([])]) });
  return { out, helper, main, lines };
}

function helperInput(helper: ScriptedModel): string {
  return helper.calls[0]!.messages.flatMap(m => m.content.flatMap(c => (c.type === 'textBlock' ? [c.text] : []))).join('\n');
}

function say(text: string) {
  return (r: TurnRequest) => {
    r.input = { kind: 'message', message: { id: 'msg_user_1', role: 'user', content: text } };
  };
}

describe('extractWindowMs', () => {
  it('caps extraction at 8 s', () => {
    expect(EXTRACT_MAX_MS).toBe(8_000);
    expect(extractWindowMs(60_000)).toBe(8_000);
  });

  it('ends extraction 5 s before the turn deadline', () => {
    expect(EXTRACT_MARGIN_MS).toBe(5_000);
    expect(extractWindowMs(10_000)).toBe(5_000);
  });

  it('skips extraction when under 2 s would remain', () => {
    expect(EXTRACT_MIN_MS).toBe(2_000);
    expect(extractWindowMs(7_000)).toBe(2_000);
    expect(extractWindowMs(6_999)).toBeUndefined();
  });
});

describe('memory extraction', () => {
  it('extracts facts from the leg user text on the helper and returns them masked', async () => {
    const r = await run(say('call me on 08123456789 after 6pm'), [factsStep(['My number is 08123456789', 'Prefers calls after 6pm'])]);
    expect(r.out.status).toBe('completed');
    expect(r.out.memory).toEqual({ facts: ['My number is [PHONE]', 'Prefers calls after 6pm'] });
    expect(r.out.usage.helper).toEqual({ inputTokens: 40, outputTokens: 6, cacheReadInputTokens: 0, cacheWriteInputTokens: 0 });
    expect(r.out.usage.inputTokens).toBe(100);
  });

  it('reads only this leg new user text, not history, a tool result, assistant text or the injected blocks', async () => {
    const r = await run((x) => {
      x.memory = { facts: FACTS, extract: MemoryExtract.FACTS };
      x.knowledge = PASSAGES;
      say('my email is budi@example.com, record it')(x);
    }, [factsStep([])], [
      { toolUse: { name: 'yasaku__record_expense', id: 'c1', input: { amount: 1 } } },
      { text: ['Recorded your expense.'] },
    ], 'rebuild_from_history');
    const seen = helperInput(r.helper);
    expect(seen).toBe(`<message>\n${escapeToolOutput(maskText('my email is budi@example.com, record it', ALL))}\n</message>`);
    for (const absent of ['record lunch', 'exp_1', 'Recorded', 'Refunds take', 'Prefers WhatsApp', '<memory>', 'tool_output', 'budi@example.com']) {
      expect(seen).not.toContain(absent);
    }
  });

  it('builds the extraction helper with an output cap of 1500 tokens', async () => {
    const helper = new ScriptedModel([factsStep(['A fact'])]);
    const models = stubModels(helper);
    const request = goldenRequest('plain_reply', (r) => {
      r.memory = { facts: [], extract: MemoryExtract.FACTS };
    });
    await executeTurn(request, never, { model: new ScriptedModel([{ text: ['ok'] }]), models, log: quiet, openMcp: fakeMcp([]) });
    expect(EXTRACT_OUTPUT_TOKENS).toBe(1500);
    expect(models.model).toHaveBeenCalledWith(request.ids.project, { ...request.agent.model, maxTokens: EXTRACT_OUTPUT_TOKENS }, request.agent.temperature);
  });

  it('cuts each fact to 500 runes, drops blanks and duplicates, and keeps at most 10', async () => {
    const many = Array.from({ length: 12 }, (_, i) => `Fact ${i}`);
    const r = await run(say('lots'), [factsStep(['😀'.repeat(600), '  ', 'Fact 0', ...many])]);
    const facts = r.out.memory!.facts!;
    expect(MAX_NEW_FACTS).toBe(10);
    expect(facts).toHaveLength(10);
    expect([...facts[0]!]).toHaveLength(500);
    expect(facts.slice(1)).toEqual(many.slice(0, 9));
  });

  it('returns masked user texts without a model call in userTexts mode', async () => {
    const r = await run((x) => {
      x.memory = { facts: [], extract: MemoryExtract.USER_TEXTS };
      say('key oagent_Ab3dEf6hIj9kLm2nOp5qRs8tUv1wXy4zAb7cDe0fGh2 and 08123456789')(x);
    }, []);
    expect(r.helper.calls).toHaveLength(0);
    expect(r.out.memory).toEqual({ userTexts: ['key [SECRET] and [PHONE]'] });
    expect(r.out.usage.helper).toBeUndefined();
  });

  it('cuts each user text to 4000 runes', async () => {
    const r = await run((x) => {
      x.memory = { facts: [], extract: MemoryExtract.USER_TEXTS };
      say('a'.repeat(5000))(x);
    }, []);
    expect(r.out.memory!.userTexts![0]).toHaveLength(4000);
  });

  it('makes no helper call and emits no memory key in none mode, but still injects the recalled facts', async () => {
    const r = await run((x) => {
      x.memory = { facts: FACTS, extract: MemoryExtract.NONE };
    }, []);
    expect(r.helper.calls).toHaveLength(0);
    expect(r.out.memory).toBeUndefined();
    expect(JSON.stringify(r.main.calls[0]!.messages)).toContain('Prefers WhatsApp after 6pm.');
  });

  it.each([
    [MemoryExtract.FACTS, 'facts', 'userTexts'],
    [MemoryExtract.USER_TEXTS, 'userTexts', 'facts'],
  ] as const)('emits the terminal shape of the %s mode and never the other key', async (mode, key, other) => {
    const r = await run((x) => {
      x.memory = { facts: [], extract: mode };
    }, [factsStep(['A fact'])]);
    expect(Object.keys(r.out.memory!)).toEqual([key]);
    expect(r.out.memory).not.toHaveProperty(other);
  });

  it.each([
    ['the helper fails', [{ error: new Error('upstream 500') }] as Step[], 'error'],
    ['the helper never returns structured output', [{ text: ['no'] }, { text: ['still no'] }] as Step[], 'error'],
    ['the helper returns the wrong shape', [factsStep('not a list')] as Step[], 'error'],
  ])('emits an empty memory and keeps the turn when %s', async (_name, steps, code) => {
    const r = await run(say('hello'), steps);
    expect(r.out.status).toBe('completed');
    expect(r.out.memory).toEqual({});
    expect(r.lines.some(l => l.includes(`"extractSkip":"${code}"`))).toBe(true);
  });

  it('aborts extraction at its deadline and keeps the turn', async () => {
    const r = await run((x) => {
      x.limits.deadlineMs = EXTRACT_MARGIN_MS + EXTRACT_MIN_MS + 100;
    }, [{ text: ['thinking'], hang: true }]);
    expect(r.out.status).toBe('completed');
    expect(r.out.memory).toEqual({});
    expect(r.lines.some(l => l.includes('"extractSkip":"deadline"'))).toBe(true);
  }, 10_000);

  it('skips extraction when too little deadline time is left', async () => {
    const r = await run((x) => {
      x.limits.deadlineMs = EXTRACT_MARGIN_MS + EXTRACT_MIN_MS - 1;
    }, [factsStep(['x'])]);
    expect(r.helper.calls).toHaveLength(0);
    expect(r.out.memory).toEqual({});
    expect(r.lines.some(l => l.includes('"extractSkip":"deadline"'))).toBe(true);
  });

  it('skips extraction when the estimate does not fit the budget', async () => {
    const r = await run((x) => {
      x.limits.budget = { remainingMicros: 10_500, prices };
    }, [factsStep(['x'])]);
    expect(r.helper.calls).toHaveLength(0);
    expect(r.out.memory).toEqual({});
    expect(r.lines.some(l => l.includes('"extractSkip":"budget"'))).toBe(true);
  });

  it('extracts under a budget that fits', async () => {
    const r = await run((x) => {
      x.limits.budget = { remainingMicros: 1_000_000, prices };
    }, [factsStep(['A fact'])]);
    expect(r.out.memory).toEqual({ facts: ['A fact'] });
  });

  it('skips extraction on a failed turn', async () => {
    const r = await run(say('hello'), [factsStep(['x'])], [{ error: new Error('upstream 500') }]);
    expect(r.out.status).toBe('failed');
    expect(r.helper.calls).toHaveLength(0);
    expect(r.out.memory).toEqual({});
    expect(r.lines.some(l => l.includes('"extractSkip":"status"'))).toBe(true);
  });

  it('extracts on an awaiting-approval leg', async () => {
    const helper = new ScriptedModel([factsStep(['Records lunch expenses'])]);
    const tool = new FunctionTool({
      name: 'yasaku__record_expense',
      description: 'Record an expense.',
      inputSchema: { type: 'object', properties: { amount: { type: 'number' }, confirm: { type: 'boolean' } } } as never,
      callback: () => ({ id: 'exp_1' }),
    });
    const request = goldenRequest('approval_interrupt', (r) => {
      r.memory = { facts: [], extract: MemoryExtract.FACTS };
    });
    const out = await executeTurn(request, never, {
      model: new ScriptedModel([{ toolUse: { name: 'yasaku__record_expense', id: 'call_1', input: { amount: 25000, confirm: true } } }]),
      models: stubModels(helper),
      log: quiet,
      openMcp: fakeMcp([tool]),
    });
    expect(out.status).toBe('awaiting_approval');
    expect(out.memory).toEqual({ facts: ['Records lunch expenses'] });
  });

  it('makes no helper call for a leg without user text', async () => {
    const r = await run((x) => {
      const message = x.input.kind === 'message' ? x.input.message : undefined;
      x.input = { kind: 'message', message: { ...message!, content: (message!.content as { type: string }[]).filter(p => p.type === 'image') } as never };
    }, [factsStep(['x'])], undefined, 'image_input');
    expect(r.helper.calls).toHaveLength(0);
    expect(r.out.memory).toEqual({});
    expect(r.lines.some(l => l.includes('"extracted":0'))).toBe(true);
  });

  it('fails open without a model source for the helper', async () => {
    const lines: string[] = [];
    const out = await executeTurn(goldenRequest('plain_reply', (r) => {
      r.memory = { facts: [], extract: MemoryExtract.FACTS };
    }), never, { model: new ScriptedModel([{ text: ['ok'] }]), log: captureLogger(lines), openMcp: fakeMcp([]) });
    expect(out.status).toBe('completed');
    expect(out.memory).toEqual({});
    expect(lines.some(l => l.includes('"extractSkip":"error"'))).toBe(true);
  });

  it.each([
    ['answers in text twice', [{ text: ['no'] }, { text: ['still no'] }, factsStep(['late']), factsStep(['later'])] as Step[]],
    ['keeps calling another tool', Array.from({ length: 4 }, (_, i) => ({ toolUse: { name: 'search', id: `t${i}`, input: {} } })) as Step[]],
  ])('calls the helper at most twice when it %s', async (_name, steps) => {
    const r = await run(say('hello'), steps);
    expect(r.helper.calls).toHaveLength(EXTRACT_MAX_CALLS);
    expect(r.out.memory).toEqual({});
  });

  it('reports a helper refused over budget as a budget skip', async () => {
    const r = await run((x) => {
      x.limits.budget = { remainingMicros: 16_000, prices };
    }, [{ text: ['Thinking.'], usage: { inputTokens: 1_000, outputTokens: 1_500 } }, factsStep(['never reached'])]);
    expect(r.helper.calls).toHaveLength(1);
    expect(r.out.status).toBe('completed');
    expect(r.out.memory).toEqual({});
    expect(r.lines.some(l => l.includes('"extractSkip":"budget"'))).toBe(true);
  });

  it('keeps the turn when anything in memory handling throws, even the log', async () => {
    const reference = await run(say('hello'), [factsStep(['A fact'])]);
    const lines: string[] = [];
    const log = captureLogger(lines);
    const info = log.info.bind(log);
    log.info = ((obj: unknown, msg?: string) => {
      if (msg === 'memory') {
        throw new Error('log sink down');
      }
      return info(obj as object, msg);
    }) as typeof log.info;
    const out = await executeTurn(goldenRequest('plain_reply', (x) => {
      x.memory = { facts: [], extract: MemoryExtract.FACTS };
      x.guardrails = { keyPrefix: 'oagent_' };
      say('hello')(x);
    }), never, {
      model: new ScriptedModel([{ text: ['Noted.'], usage: { inputTokens: 100, outputTokens: 3 } }]),
      models: stubModels(new ScriptedModel([factsStep(['A fact'])])),
      log,
      openMcp: fakeMcp([recordTool([])]),
    });
    expect(out.status).toBe('completed');
    expect(out.error).toBeUndefined();
    expect(out.memory).toEqual({});
    expect(out.usage).toEqual(reference.out.usage);
    expect(out.newMessages.map(m => m.content.map(c => (c.type === 'textBlock' ? c.text : c.type)))).toEqual(reference.out.newMessages.map(m => m.content.map(c => (c.type === 'textBlock' ? c.text : c.type))));
  });

  it('never logs fact or user text', async () => {
    const r = await run(say('secret plan: budi likes durian'), [factsStep(['Likes durian'])]);
    const logged = r.lines.join('\n');
    expect(logged).toContain('"extracted":1');
    expect(logged).not.toContain('durian');
  });
});

describe('the terminal memory field', () => {
  async function terminal(over: (r: TurnRequest) => void, helperSteps: Step[]): Promise<Record<string, unknown>> {
    const events: Record<string, unknown>[] = [];
    const runTurn = createRunTurn({ models: stubModels(new ScriptedModel(helperSteps)), log: quiet, modelFor: () => new ScriptedModel([{ text: ['ok'] }]), openMcp: fakeMcp([]) });
    await runTurn(goldenRequest('plain_reply', over), async (e) => {
      events.push(JSON.parse(JSON.stringify(e)) as Record<string, unknown>);
    }, never);
    return events.find(e => e.type === 'OAGENT_TERMINAL')!;
  }

  it('emits no memory field when the request has none', async () => {
    const t = await terminal(() => {}, []);
    expect(Object.keys(t)).toEqual(['type', 'status', 'stopReason', 'snapshot', 'usage', 'messagesAppended', 'interrupts', 'notes']);
  });

  it('emits the memory field when the request carried memory', async () => {
    const t = await terminal((r) => {
      r.memory = { facts: [], extract: MemoryExtract.FACTS };
    }, [factsStep(['A fact'])]);
    expect(t.memory).toEqual({ facts: ['A fact'] });
    expect((t.usage as Record<string, unknown>).helper).toBeDefined();
  });

  it('keeps no memory between requests', async () => {
    const main = new ScriptedModel([{ text: ['ok'] }, { text: ['ok'] }]);
    const helper = new ScriptedModel([factsStep(['A fact'])]);
    const runTurn = createRunTurn({ models: stubModels(helper), log: quiet, modelFor: () => main, openMcp: fakeMcp([]) });
    const terminals: Record<string, unknown>[] = [];
    const emit = async (e: Record<string, unknown>) => {
      if (e.type === 'OAGENT_TERMINAL') {
        terminals.push(JSON.parse(JSON.stringify(e)) as Record<string, unknown>);
      }
    };
    await runTurn(goldenRequest('plain_reply', (r) => {
      r.memory = { facts: FACTS, extract: MemoryExtract.FACTS };
    }), emit, never);
    await runTurn(goldenRequest('plain_reply'), emit, never);
    const second = JSON.stringify(main.calls[1]);
    expect(second).not.toContain('<memory>');
    expect(second).not.toContain('Prefers WhatsApp');
    expect(second).not.toContain('earlier conversation');
    expect(terminals[1]).not.toHaveProperty('memory');
    expect(helper.calls).toHaveLength(1);
  });
});
