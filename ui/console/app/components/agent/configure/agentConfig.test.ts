import type { AgentConfig, WebToolsConfig } from './agentConfig';
import { describe, expect, it } from 'vitest';
import { changes, emptyConfig, fromWire, limitsOf, toWire } from './agentConfig';

const names = { provider: (id: string) => id, server: (id: string) => id };
const money = (c: number) => String(c);
const web: WebToolsConfig = { search: true, fetch: false, http: true, fetchAllowDomains: [], httpHosts: ['api.example.com'], requestHeaders: ['X-Trace'], timeoutMs: 20000, maxCallsPerTurn: 5 };
const wire = { ...toWire(emptyConfig()), providerMissing: false, tools: [], mcpServers: [] };

describe('web tools in the agent config', () => {
  it('reads an old config without web tools as all off', () => {
    expect(fromWire(wire).web).toEqual(emptyConfig().web);
    expect(emptyConfig().web).toEqual({ search: false, fetch: false, http: false, fetchAllowDomains: [], httpHosts: [], requestHeaders: [], timeoutMs: 0, maxCallsPerTurn: 0 });
  });

  it('round-trips the web tools and always sends them', () => {
    const config: AgentConfig = fromWire({ ...wire, web });
    expect(config.web).toEqual(web);
    expect(config.web.httpHosts).not.toBe(web.httpHosts);
    expect(toWire(config).web).toEqual(web);
    expect(toWire(emptyConfig()).web).toEqual(emptyConfig().web);
  });

  it('lists each web change', () => {
    const live = emptyConfig();
    const on = { ...live, web };
    expect(changes(on, live, money, names)).toEqual([
      { key: 'configure.change_web_on', params: { tool: 'web_search' } },
      { key: 'configure.change_web_on', params: { tool: 'http_request' } },
      { key: 'configure.change_web_lists' },
      { key: 'configure.change_web_limits' },
    ]);
    expect(changes(live, on, money, names).slice(0, 2)).toEqual([
      { key: 'configure.change_web_off', params: { tool: 'web_search' } },
      { key: 'configure.change_web_off', params: { tool: 'http_request' } },
    ]);
    expect(changes({ ...on, web: { ...web, fetch: true } }, on, money, names)).toEqual([{ key: 'configure.change_web_on', params: { tool: 'web_fetch' } }]);
    expect(changes({ ...on, web: { ...web, fetchAllowDomains: ['bca.co.id'] } }, on, money, names)).toEqual([{ key: 'configure.change_web_lists' }]);
    expect(changes({ ...on, web: { ...web, requestHeaders: [] } }, on, money, names)).toEqual([{ key: 'configure.change_web_lists' }]);
    expect(changes({ ...on, web: { ...web, maxCallsPerTurn: 6 } }, on, money, names)).toEqual([{ key: 'configure.change_web_limits' }]);
    expect(changes({ ...on, web: { ...web, timeoutMs: 9000 } }, on, money, names)).toEqual([{ key: 'configure.change_web_limits' }]);
    expect(changes(on, on, money, names)).toEqual([]);
  });

  it('reads the web limits from the agent defaults, leaving out 0 or missing', () => {
    const d = { maxSteps: 0, maxMaxSteps: 0, maxTemperature: 0, maxCostCents: 0, voiceNoteMaxSeconds: 0, voiceNoteMaxBytes: 0, webTimeoutMs: 15000, maxWebTimeoutMs: 30000, webMaxCallsPerTurn: 10, maxWebMaxCallsPerTurn: 0 };
    expect(limitsOf(d)).toMatchObject({ webTimeoutMs: 15000, maxWebTimeoutMs: 30000, webMaxCallsPerTurn: 10, maxWebMaxCallsPerTurn: undefined });
    expect(limitsOf().webTimeoutMs).toBeUndefined();
    expect(limitsOf({ ...d, webTimeoutMs: 0 }).webTimeoutMs).toBeUndefined();
  });
});
