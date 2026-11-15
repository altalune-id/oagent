import type { TurnRequest } from '../contract/index.ts';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';

import { goldenRequest } from '../test/turn-fixtures.ts';
import { WEB_APPROVAL, webApprovalSpecs } from './web.ts';

function withWeb(web: Partial<NonNullable<TurnRequest['web']>>): TurnRequest {
  return goldenRequest('plain_reply', (r) => {
    r.web = { timeoutMs: 15_000, maxCallsPerTurn: 10, ...web };
  });
}

describe('web approval specs', () => {
  it('fixes the built-in specs Go asserts against', () => {
    const shared = JSON.parse(readFileSync(fileURLToPath(new URL('../../../../internal/kernel/webpolicy/testdata/approval.json', import.meta.url)), 'utf8')) as unknown;
    expect(WEB_APPROVAL).toEqual(shared);
  });

  it('gates only the tools the request carries, under the web_tools server', () => {
    expect(webApprovalSpecs(goldenRequest('plain_reply'))).toEqual(new Map());
    expect(webApprovalSpecs(withWeb({ search: { kind: 'brave', apiKey: 'k' }, http: { hosts: ['api.example.com'], requestHeaders: [], secrets: [] } }))).toEqual(new Map([
      ['web_search', { server: 'web_tools', tool: 'web_search', approval: { mode: 'never' } }],
      ['http_request', { server: 'web_tools', tool: 'http_request', approval: { mode: 'conditional', freeWhen: [{ arg: 'method', values: ['GET'] }] } }],
    ]));
    expect([...webApprovalSpecs(withWeb({ fetch: { allowDomains: [] } })).keys()]).toEqual(['web_fetch']);
  });
});
