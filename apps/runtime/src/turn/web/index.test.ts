import type { TurnRequest } from '../../contract/index.ts';
import { describe, expect, it } from 'vitest';

import { ScriptedModel } from '../../test/scripted-model.ts';
import { goldenRequest, quiet } from '../../test/turn-fixtures.ts';
import { emptyUsage } from '../limits.ts';
import { WEB_LINE, webTools } from './index.ts';

type Web = NonNullable<TurnRequest['web']>;

function withWeb(web: Partial<Web>, name = 'plain_reply'): TurnRequest {
  return goldenRequest(name, (r) => {
    r.web = { timeoutMs: 15_000, maxCallsPerTurn: 10, ...web };
  });
}

const search = { kind: 'brave' as const, apiKey: 'brave-key' };
const http = { hosts: ['api.example.com'], requestHeaders: [], secrets: [] };
const models = { model: () => new ScriptedModel([]) };

describe('webTools', () => {
  it('builds nothing without a web block', () => {
    expect(webTools(goldenRequest('plain_reply'), { usage: emptyUsage(), models, log: quiet })).toEqual({ tools: [], promptLine: undefined });
  });

  it('builds the tools the request carries and the prompt line', () => {
    const built = webTools(withWeb({ search, fetch: { allowDomains: [] }, http }), { usage: emptyUsage(), models, log: quiet });
    expect(built.tools.map(t => t.name)).toEqual(['web_search', 'web_fetch', 'http_request']);
    expect(built.promptLine).toBe(WEB_LINE);
  });

  it('refuses to build web_fetch without models', () => {
    expect(() => webTools(withWeb({ fetch: { allowDomains: [] } }), { usage: emptyUsage(), log: quiet })).toThrow('web_fetch needs the model factory');
  });
});
