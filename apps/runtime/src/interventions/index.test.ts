import type { TurnRequest } from '../contract/index.ts';
import { describe, expect, it } from 'vitest';

import { TurnRefusedError } from '../contract/index.ts';
import { goldenRequest, oldGoRequest } from '../test/turn-fixtures.ts';
import { approvalSpecs, checkApprovalReady, interventionsFor } from './index.ts';

function v3(mode: 'never' | 'always'): TurnRequest {
  return goldenRequest('tool_call', (r) => {
    r.contractVersion = 3;
    const server = r.mcpServers[0] as unknown as Record<string, unknown>;
    delete server.allowedTools;
    server.tools = [{ name: 'record_expense', approval: { mode } }];
  });
}

describe('intervention order', () => {
  it('registers oagent:approval, the only handler so far', () => {
    expect(interventionsFor(approvalSpecs(v3('always'))).map(h => h.name)).toEqual(['oagent:approval']);
  });

  it('maps each granted tool to its approval spec under its model name', () => {
    expect(approvalSpecs(v3('always'))).toEqual(new Map([['yasaku__record_expense', { server: 'yasaku', tool: 'record_expense', approval: { mode: 'always' } }]]));
    expect(approvalSpecs(oldGoRequest('tool_call_v2'))).toEqual(new Map([['yasaku__record_expense', { server: 'yasaku', tool: 'record_expense', approval: { mode: 'never' } }]]));
  });

  it('refuses a gated tool when oagent:approval is not registered (422)', () => {
    expect(() => checkApprovalReady(v3('always'), [])).toThrow(TurnRefusedError);
    expect(() => checkApprovalReady(v3('always'), [])).toThrow('approval_unavailable');
    expect(() => checkApprovalReady(v3('never'), [])).not.toThrow();
    expect(() => checkApprovalReady(v3('always'), interventionsFor(approvalSpecs(v3('always'))))).not.toThrow();
  });
});
