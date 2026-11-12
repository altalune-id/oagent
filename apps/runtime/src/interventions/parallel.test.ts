import type { TurnRequest } from '../contract/index.ts';
import type { ToolResultReport } from '../turn/execute.ts';
import { FunctionTool } from '@strands-agents/sdk';
import { describe, expect, it } from 'vitest';

import { ScriptedModel } from '../test/scripted-model.ts';
import { fakeMcp, goldenRequest, quiet } from '../test/turn-fixtures.ts';
import { executeTurn } from '../turn/execute.ts';

const never = new AbortController().signal;

function tools(ran: string[]): FunctionTool[] {
  const make = (name: string) => new FunctionTool({
    name: `yasaku__${name}`,
    description: name,
    inputSchema: { type: 'object', properties: { amount: { type: 'number' } } },
    callback: () => {
      ran.push(name);
      return { ok: name };
    },
  });
  return [make('record_expense'), make('record_income'), make('list_wallets')];
}

function request(over: (r: TurnRequest) => void = () => {}): TurnRequest {
  return goldenRequest('tool_call', (r) => {
    r.contractVersion = 3;
    const server = r.mcpServers[0] as unknown as Record<string, unknown>;
    delete server.allowedTools;
    server.tools = [
      { name: 'record_expense', approval: { mode: 'always' } },
      { name: 'record_income', approval: { mode: 'always' } },
      { name: 'list_wallets', approval: { mode: 'never' } },
    ];
    over(r);
  });
}

const step = { toolUses: [
  { name: 'yasaku__record_expense', id: 'c1', input: { amount: 1 } },
  { name: 'yasaku__record_income', id: 'c2', input: { amount: 2 } },
  { name: 'yasaku__list_wallets', id: 'c3', input: {} },
] };

describe('parallel gated calls in one model step', () => {
  it('stop once with an interrupt for every gated sibling, run none of them, and run the free sibling once', async () => {
    const ran: string[] = [];
    const checkpoints: ToolResultReport[] = [];
    const first = await executeTurn(request(), never, {
      model: new ScriptedModel([step]),
      log: quiet,
      openMcp: fakeMcp(tools(ran)),
      onToolResult: async (r) => {
        checkpoints.push(r);
      },
    });
    expect(first.status).toBe('awaiting_approval');
    expect(first.interrupts.map(i => i.id).sort()).toEqual(['hook:beforeToolCall:c1:oagent:approval', 'hook:beforeToolCall:c2:oagent:approval']);
    expect(ran).toEqual(['list_wallets']);

    const hashes = Object.fromEntries(first.interrupts.map(i => [i.id, (i.reason as { argsHash: string }).argsHash]));
    const second = await executeTurn(request((r) => {
      r.snapshot = first.snapshot as never;
      r.storedResults = checkpoints.map(c => ({ toolUseId: c.toolUseId, result: c.result })) as never;
      r.input = { kind: 'resume', responses: first.interrupts.map(i => ({ interruptId: i.id, response: { approved: true, text: '', argsHash: hashes[i.id] } })) };
    }), never, { model: new ScriptedModel([{ text: ['All recorded.'] }]), log: quiet, openMcp: fakeMcp(tools(ran)) });
    expect(second.status).toBe('completed');
    expect(ran.sort()).toEqual(['list_wallets', 'record_expense', 'record_income']);
  });
});
