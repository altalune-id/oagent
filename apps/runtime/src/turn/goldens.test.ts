import type { TurnRequest } from '../contract/index.ts';
import type { Step } from '../test/scripted-model.ts';
import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { FunctionTool } from '@strands-agents/sdk';
import { describe, expect, it } from 'vitest';

import { createModels } from '../models/index.ts';
import { ScriptedModel } from '../test/scripted-model.ts';
import { fakeMcp, goldenRequest, pendingSnapshot, quiet } from '../test/turn-fixtures.ts';
import { createRunTurn } from './index.ts';
import { SDK_VERSION } from './snapshot.ts';

const dir = fileURLToPath(new URL('../../../../internal/kernel/runtime/testdata/turn/', import.meta.url));
const appData = { thread: 't0193a2b4c5d67e8f9a0b1c2d3e4f5a6b', agentVersion: 3, sdk: SDK_VERSION };
const args = { amount: 25000, account: 'cash', note: 'lunch' };
const callArgs = { name: 'yasaku__record_expense', id: 'call_1', input: args };

const withConfirm = { type: 'object', properties: { amount: { type: 'number' }, account: { type: 'string' }, note: { type: 'string' }, confirm: { type: 'boolean' } } };
const withDryRun = { type: 'object', properties: { amount: { type: 'number' }, account: { type: 'string' }, note: { type: 'string' }, dry_run: { type: 'boolean' } } };
const withoutConfirm = { type: 'object', properties: { amount: { type: 'number' }, account: { type: 'string' }, note: { type: 'string' } } };

function expenseTool(schema: Record<string, unknown> = { type: 'object' }): FunctionTool {
  return new FunctionTool({
    name: 'yasaku__record_expense',
    description: 'Record an expense.',
    inputSchema: schema as never,
    callback: () => ({ id: 'exp_1', amount: 25000, account: 'cash' }),
  });
}

type Case = {
  steps: Step[],
  schema?: Record<string, unknown>,
  pending?: boolean,
  down?: boolean,
};

const cases: Record<string, Case> = {
  plain_reply: { steps: [{ text: ['Hi, ', 'how can I help?'], usage: { inputTokens: 120, outputTokens: 9 } }] },
  anthropic_reply: { steps: [{ text: ['Hi, ', 'how can I help?'], usage: { inputTokens: 120, outputTokens: 9 } }] },
  tool_call: { steps: [
    { toolUse: callArgs, usage: { inputTokens: 450, outputTokens: 40 } },
    { text: ['Recorded lunch: 25,000 from cash.'], usage: { inputTokens: 460, outputTokens: 24 } },
  ] },
  mcp_server_down: { down: true, steps: [{ text: ['I can\'t record expenses right now: the expense tools are unavailable.'], usage: { inputTokens: 140, outputTokens: 18 } }] },
  resume: { pending: true, steps: [{ text: ['Done.'], usage: { inputTokens: 300, outputTokens: 3 } }] },
  stored_result: { pending: true, steps: [{ text: ['Recorded lunch: 25,000 from cash.'], usage: { inputTokens: 905, outputTokens: 12 } }] },
  rebuild_from_history: { steps: [{ text: ['Lunch, 25,000 from cash.'], usage: { inputTokens: 150, outputTokens: 7 } }] },
  limit_reached: { steps: [{ toolUse: callArgs, usage: { inputTokens: 890, outputTokens: 40 } }] },
  model_error: { steps: [{ toolUse: callArgs, usage: { inputTokens: 880, outputTokens: 30 } }, { error: new Error('upstream 500') }] },
  cancel: { steps: [{ text: ['Let me think'], hang: true }] },
  resume_after_crash: { steps: [{ text: ['Recorded lunch: 25,000 from cash.'], usage: { inputTokens: 460, outputTokens: 12 } }] },
  image_input: { steps: [{ text: ['A receipt: lunch, 25,000.'], usage: { inputTokens: 900, outputTokens: 10 } }] },
  image_refused: { steps: [{ error: Object.assign(new Error('this model does not support image input'), { status: 400 }) }] },
  approval_absent_only: { schema: withDryRun, steps: [
    { toolUse: callArgs, usage: { inputTokens: 450, outputTokens: 40 } },
    { text: ['Recorded lunch: 25,000 from cash.'], usage: { inputTokens: 460, outputTokens: 24 } },
  ] },
  approval_interrupt: { schema: withConfirm, steps: [{ toolUse: { ...callArgs, input: { ...args, confirm: true } }, usage: { inputTokens: 450, outputTokens: 40 } }] },
  approval_rule_broken: { schema: withoutConfirm, steps: [{ toolUse: { ...callArgs, input: { ...args, confirm: false } }, usage: { inputTokens: 450, outputTokens: 40 } }] },
  approval_resume_approved: { pending: true, schema: withoutConfirm, steps: [{ text: ['Recorded lunch: 25,000 from cash.'], usage: { inputTokens: 905, outputTokens: 12 } }] },
  approval_resume_rejected: { pending: true, schema: withoutConfirm, steps: [{ text: ['Understood, I will use 20,000 instead.'], usage: { inputTokens: 910, outputTokens: 11 } }] },
  approval_hash_mismatch: { pending: true, schema: withoutConfirm, steps: [{ text: ['That approval did not match, so nothing was recorded.'], usage: { inputTokens: 905, outputTokens: 12 } }] },
};

type Event = Record<string, unknown>;

function normalize(events: Event[]): Event[] {
  return events.filter(e => e.type !== 'OAGENT_HEARTBEAT').map((e) => {
    if (e.type !== 'OAGENT_TERMINAL') {
      return e;
    }
    const usage = { ...(e.usage as Record<string, unknown>) };
    if (usage.estimated) {
      usage.inputTokens = '<estimated>';
    }
    return { ...e, snapshot: e.snapshot === null ? null : '<snapshot>', usage };
  });
}

function goldenStream(name: string): Event[] {
  return readFileSync(join(dir, `${name}.stream.ndjson`), 'utf8').trim().split('\n').map(l => JSON.parse(l) as Event);
}

describe('golden replay', () => {
  it('has a script for every golden', () => {
    const names = readdirSync(dir).filter(f => f.endsWith('.stream.ndjson')).map(f => f.replace('.stream.ndjson', '')).sort();
    expect(Object.keys(cases).sort()).toEqual(names);
  });

  it.each(Object.keys(cases))('replays %s', async (name) => {
    const c = cases[name]!;
    const snapshot = c.pending ? await pendingSnapshot('oagent:approval', appData, args) : null;
    const request: TurnRequest = goldenRequest(name, (r) => {
      if (c.pending) {
        r.snapshot = snapshot as never;
      }
    });
    const events: Event[] = [];
    const runTurn = createRunTurn({
      models: createModels(),
      log: quiet,
      modelFor: () => new ScriptedModel(c.steps),
      openMcp: fakeMcp(c.down ? [] : [expenseTool(c.schema)], c.down
        ? { notes: [{ kind: 'mcp_server_unavailable', server: 'yasaku' }], promptLines: ['Tools from yasaku are unavailable in this turn.'] }
        : {}),
    });
    await runTurn(request, async (e) => {
      events.push(JSON.parse(JSON.stringify(e)) as Event);
    }, new AbortController().signal);
    expect(normalize(events)).toEqual(normalize(goldenStream(name)));
  });
});
