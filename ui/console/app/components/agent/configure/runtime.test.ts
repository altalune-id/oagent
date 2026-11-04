import type { AgentRuntimeConfig } from '~/stores/agentRuntime';
import type { McpServer } from '~/stores/mcpServers';
import { describe, expect, it } from 'vitest';
import { sampleAgentRuntime, sampleKnowledgeSources, sampleMcpServers, sampleProviders } from '~/preview/samples';
import { emptyRuntime } from '~/stores/agentRuntime';
import {
  booleanArgs,
  costCapError,
  defaultAsk,
  embeddingReady,
  grantFor,
  missingServers,
  parseDollars,
  parseDomains,
  publishBlockers,
  readsPrivateData,
  readySources,
  runtimeChanges,
  withAsk,
  withGrant,
  withMatch,
} from './runtime';

const providers = sampleProviders();
const [yasaku, tagihan] = sampleMcpServers() as [McpServer, McpServer];
const tool = (name: string) => yasaku.tools.find(t => t.name === name)!;
const ctx = { providers, servers: [yasaku, tagihan], maxCostCents: 20, oldTools: 0 };

describe('grants', () => {
  it('asks by default except for read-only tools on a trusted server', () => {
    expect(defaultAsk(yasaku, tool('list_wallets'))).toBe('never');
    expect(defaultAsk(yasaku, tool('record_expense'))).toBe('always');
    expect(defaultAsk(tagihan, tagihan.tools[0]!)).toBe('always');
  });

  it('turns a tool on with its default, then off', () => {
    const on = withGrant(emptyRuntime(), yasaku, tool('list_wallets'), true);
    expect(grantFor(on, yasaku.id, 'list_wallets')).toEqual({ serverId: yasaku.id, serverName: 'yasaku', tool: 'list_wallets', ask: 'never' });
    expect(withGrant(on, yasaku, tool('list_wallets'), false).grants).toEqual([]);
  });

  it('switches a match rule on the first boolean argument and back to every call', () => {
    let c = withGrant(emptyRuntime(), yasaku, tool('record_expense'), true);
    expect(booleanArgs(tool('record_expense'))).toEqual(['confirm']);
    c = withAsk(c, yasaku.id, tool('record_expense'), 'match');
    expect(grantFor(c, yasaku.id, 'record_expense')).toMatchObject({ ask: 'match', matchArg: 'confirm', matchValue: true });
    c = withMatch(c, yasaku.id, 'record_expense', 'confirm', false);
    expect(grantFor(c, yasaku.id, 'record_expense')!.matchValue).toBe(false);
    c = withAsk(c, yasaku.id, tool('record_expense'), 'match');
    expect(grantFor(c, yasaku.id, 'record_expense')!.matchValue).toBe(false);
    c = withAsk(c, yasaku.id, tool('record_expense'), 'always');
    expect(grantFor(c, yasaku.id, 'record_expense')).toEqual({ serverId: yasaku.id, serverName: 'yasaku', tool: 'record_expense', ask: 'always' });
    expect(withAsk(c, 'other', tool('record_expense'), 'never')).toEqual(c);
    expect(withMatch(c, 'other', 'record_expense', 'x', true)).toEqual(c);
  });

  it('finds servers the grants name but the project lost', () => {
    const c = sampleAgentRuntime();
    expect(missingServers(c, [yasaku])).toEqual([]);
    expect(missingServers(c, [tagihan])).toEqual([{ id: yasaku.id, name: 'yasaku', tools: 7 }]);
  });
});

describe('web, memory and knowledge', () => {
  it('counts any granted tool or memory as private data', () => {
    expect(readsPrivateData(emptyRuntime())).toBe(false);
    expect(readsPrivateData({ ...emptyRuntime(), memory: true })).toBe(true);
    expect(readsPrivateData(sampleAgentRuntime())).toBe(true);
    expect(readsPrivateData({ ...emptyRuntime(), knowledgeIds: ['kns_1'] })).toBe(true);
  });

  it('formats and reads dollar caps, keeping empty (no cap) apart from text that is not an amount', () => {
    expect(parseDollars('')).toBe(0);
    expect(parseDollars(' 0 ')).toBe(0);
    expect(parseDollars('0.2')).toBe(20);
    expect(parseDollars('$1,5')).toBe(150);
    expect(parseDollars('abc')).toBeNull();
    expect(parseDollars('0.123')).toBeNull();
  });

  it('parses one domain per line', () => {
    expect(parseDomains(' BCA.co.id\n\nmandiri.co.id \n')).toEqual(['bca.co.id', 'mandiri.co.id']);
  });

  it('needs an embedding model for knowledge and keeps only ready sources', () => {
    expect(embeddingReady(providers)).toBe(true);
    expect(embeddingReady([providers[1]!])).toBe(false);
    expect(readySources(sampleKnowledgeSources()).map(s => s.name)).toEqual(['Panduan kategori yasaku']);
  });
});

describe('publishBlockers', () => {
  it('lets the sample draft through', () => {
    expect(publishBlockers(sampleAgentRuntime(), ctx)).toEqual([]);
  });

  it('asks for a model, then names a removed one', () => {
    expect(publishBlockers(emptyRuntime(), ctx).map(b => b.key)).toEqual(['publish.blocker_model']);
    const gone = { ...sampleAgentRuntime(), model: 'openai/gpt-3' };
    expect(publishBlockers(gone, ctx)).toEqual([{ key: 'publish.blocker_model_removed', target: 'model', params: { model: 'openai/gpt-3', provider: 'OpenRouter' } }]);
  });

  it('refuses a cost cap on an unpriced model', () => {
    const groq: AgentRuntimeConfig = { ...emptyRuntime(), providerId: providers[1]!.id, model: 'llama-3.3-70b-versatile' };
    expect(costCapError(groq, ctx)).toBe('limits.unpriced');
    expect(costCapError(groq, { ...ctx, maxCostCents: 0 })).toBe('');
    expect(publishBlockers(groq, ctx)).toEqual([{ key: 'publish.blocker_unpriced', target: 'limits', params: { provider: 'Groq' } }]);
  });

  it('needs an allow-list, the granted servers and no old tools', () => {
    const open = { ...sampleAgentRuntime(), allowedDomains: [] };
    expect(publishBlockers(open, { ...ctx, servers: [tagihan], oldTools: 2 }).map(b => [b.key, b.target])).toEqual([
      ['publish.blocker_allowlist', 'web'],
      ['publish.blocker_server_missing', 'tools'],
      ['publish.blocker_legacy_tools', 'tools'],
    ]);
  });
});

describe('runtimeChanges', () => {
  it('lists everything the sample draft adds over a legacy live version', () => {
    expect(runtimeChanges(sampleAgentRuntime(), undefined, providers).map(c => c.key)).toEqual([
      'configure.change_provider_model',
      'configure.change_helper',
      'configure.change_tool_added_approval',
      'configure.change_tool_added_approval',
      'configure.change_tool_added_approval',
      'configure.change_tool_added_approval',
      'configure.change_tool_added',
      'configure.change_tool_added',
      'configure.change_tool_added',
      'configure.change_web_fetch_on',
      'configure.change_memory_on',
      'configure.change_knowledge',
    ]);
  });

  it('lists ask changes, removals and switches turned off against a live config', () => {
    const live = { ...sampleAgentRuntime(), webSearch: true };
    const draft = withAsk(live, live.grants[0]!.serverId, tool('record_expense'), 'always');
    const next = { ...draft, grants: draft.grants.filter(g => g.tool !== 'delete_tx'), webFetch: false, webSearch: false, memory: false, providerId: 'gone' };
    expect(runtimeChanges(next, live, providers)).toEqual([
      { key: 'configure.change_provider_model', params: { provider: '—', model: 'openai/gpt-4.1-mini' } },
      { key: 'configure.change_tool_ask', params: { tool: 'yasaku__record_expense' } },
      { key: 'configure.change_tool_removed', params: { tool: 'yasaku__delete_tx' } },
      { key: 'configure.change_web_fetch_off' },
      { key: 'configure.change_web_search_off' },
      { key: 'configure.change_memory_off' },
    ]);
    expect(runtimeChanges({ ...emptyRuntime(), webSearch: true }, emptyRuntime(), providers)).toEqual([{ key: 'configure.change_web_search_on' }]);
    expect(runtimeChanges({ ...emptyRuntime(), webFetch: true }, emptyRuntime(), providers)).toEqual([]);
    expect(runtimeChanges({ ...emptyRuntime(), webFetch: true, allowedDomains: ['bca.co.id'] }, emptyRuntime(), providers)).toEqual([{ key: 'configure.change_web_fetch_on', params: { domains: 'bca.co.id' } }]);
  });
});
