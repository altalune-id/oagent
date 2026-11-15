import type { TurnRequest } from '../contract/index.ts';
import { describe, expect, it } from 'vitest';

import { TurnRefusedError } from '../contract/index.ts';
import { goldenRequest, oldGoRequest } from '../test/turn-fixtures.ts';
import { webTurn } from '../test/web.ts';
import { httpTool } from '../turn/web/http.ts';
import { approvalSpecs, checkApprovalReady, interventionsFor } from './index.ts';
import { evaluate } from './rule.ts';

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

  it('adds no web spec to a request without web', () => {
    expect([...approvalSpecs(v3('always')).keys()]).toEqual(['yasaku__record_expense']);
  });

  it('merges the web specs; http_request is free on GET only', () => {
    const request = v3('never');
    request.web = { timeoutMs: 15_000, maxCallsPerTurn: 10, http: { hosts: ['api.example.com'], requestHeaders: [], secrets: [] } };
    const gate = approvalSpecs(request).get('http_request')!;
    const schema = httpTool(webTurn(), request.web.http!).toolSpec.inputSchema;
    expect(evaluate(gate.approval, schema, { method: 'GET', url: 'https://api.example.com/' })).toBe('free');
    for (const method of ['POST', 'PUT', 'PATCH', 'DELETE', 'get']) {
      expect(evaluate(gate.approval, schema, { method, url: 'https://api.example.com/' }), method).toBe('ask');
    }
    expect(() => checkApprovalReady(request, [])).toThrow('approval_unavailable');
  });
});
