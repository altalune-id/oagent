import type { AgentConfig } from './agentConfig';
import type { McpServer } from '~/stores/mcpServers';
import { describe, expect, it } from 'vitest';
import { providers as fixtureProviders, servers as fixtureServers } from '~~/test/fixtures/runtime';
import { sampleKnowledgeSources } from '~/preview/samples';
import { emptyRuntime } from '~/stores/agentRuntime';
import { emptyConfig } from './agentConfig';
import {
  chosenModel,
  costCapError,
  grantFor,
  grantState,
  helperCapError,
  missingServers,
  parseDollars,
  parseDomains,
  publishBlockers,
  readsPrivateData,
  readySources,
  withAllGrants,
  withApproval,
  withGrant,
  withoutGrant,
} from './runtime';

const providers = fixtureProviders();
const [yasaku, tagihan] = fixtureServers() as [McpServer, McpServer];
const tool = (name: string) => yasaku.tools.find(t => t.name === name)!;
const ctx = { providers, servers: [yasaku, tagihan] };
const ready = (over: Partial<AgentConfig> = {}): AgentConfig => ({ ...emptyConfig(), providerId: 'Pv6tQm2ZxR8wKp3VbNc4K', model: 'openai/gpt-4.1-mini', ...over });

describe('select all tools of a server', () => {
  const odd = { ...yasaku.tools[0]!, name: 'odd.name', grantable: false };
  const server = { ...yasaku, tools: [...yasaku.tools, odd] };
  const grantable = server.tools.filter(t => t.grantable).map(t => t.name).sort();

  it('starts with none on', () => {
    expect(grantState(emptyConfig(), server)).toBe('none');
  });

  it('turns every grantable tool on, never one that cannot be granted', () => {
    const all = withAllGrants(emptyConfig(), server, server.tools, true);
    expect(all.grants.find(g => g.serverId === server.id)!.tools.map(t => t.name).sort()).toEqual(grantable);
    expect(grantFor(all, server.id, 'odd.name')).toBeUndefined();
    expect(grantState(all, server)).toBe('all');
  });

  it('is partial with some on, and select all keeps the approval already set on those', () => {
    const first = server.tools.find(t => t.grantable)!;
    const some = withApproval(withGrant(emptyConfig(), server, first, true), server.id, first.name, !first.requireApproval);
    expect(grantState(some, server)).toBe('some');
    const all = withAllGrants(some, server, server.tools, true);
    expect(grantState(all, server)).toBe('all');
    expect(grantFor(all, server.id, first.name)!.requireApproval).toBe(!first.requireApproval);
  });

  it('turns them all off without touching another server', () => {
    const both = withGrant(withAllGrants(emptyConfig(), server, server.tools, true), tagihan, tagihan.tools[0]!, true);
    const off = withAllGrants(both, server, server.tools, false);
    expect(grantState(off, server)).toBe('none');
    expect(grantFor(off, tagihan.id, tagihan.tools[0]!.name)).toBeDefined();
  });

  it('acts only on the shown tools, leaving the hidden ones as they were', () => {
    const [a, b] = server.tools.filter(t => t.grantable);
    const on = withAllGrants(withGrant(emptyConfig(), server, b!, true), server, [a!], true);
    expect(grantState(on, server, [a!])).toBe('all');
    const off = withAllGrants(on, server, [a!], false);
    expect([!!grantFor(off, server.id, a!.name), !!grantFor(off, server.id, b!.name)]).toEqual([false, true]);
  });
});

describe('grants', () => {
  it('turns a tool on with the server\'s suggested approval, then off, dropping an empty server', () => {
    const on = withGrant(withGrant(emptyConfig(), yasaku, tool('list_wallets'), true), yasaku, tool('record_expense'), true);
    expect(grantFor(on, yasaku.id, 'list_wallets')).toEqual({ name: 'list_wallets', requireApproval: false });
    expect(grantFor(on, yasaku.id, 'record_expense')).toEqual({ name: 'record_expense', requireApproval: true });
    expect(on.grants).toHaveLength(1);
    const off = withGrant(withGrant(on, yasaku, tool('list_wallets'), false), yasaku, tool('record_expense'), false);
    expect(off.grants).toEqual([]);
    expect(grantFor(off, tagihan.id, 'list_bills')).toBeUndefined();
  });

  it('prefills approval strictly from the server\'s require_approval: an untrusted server\'s read-only tool still asks', () => {
    const listBills = tagihan.tools.find(t => t.name === 'list_bills')!;
    expect([tagihan.trusted, listBills.hint]).toEqual([false, 'read_only']);
    expect(grantFor(withGrant(emptyConfig(), tagihan, listBills, true), tagihan.id, 'list_bills')).toEqual({ name: 'list_bills', requireApproval: true });
    expect(grantFor(withGrant(emptyConfig(), tagihan, { ...listBills, requireApproval: false }, true), tagihan.id, 'list_bills')!.requireApproval).toBe(false);
  });

  it('changes one tool\'s approval and leaves the others', () => {
    const on = withGrant(withGrant(emptyConfig(), yasaku, tool('record_expense'), true), tagihan, tagihan.tools[0]!, true);
    const relaxed = withApproval(on, yasaku.id, 'record_expense', false);
    expect(grantFor(relaxed, yasaku.id, 'record_expense')!.requireApproval).toBe(false);
    expect(grantFor(relaxed, tagihan.id, 'list_bills')!.requireApproval).toBe(true);
    expect(withApproval(relaxed, yasaku.id, 'other', true)).toEqual(relaxed);
  });

  it('finds grants of servers marked missing or no longer listed, and removes them', () => {
    const config = ready({ grants: [
      { serverId: yasaku.id, missing: false, tools: [{ name: 'list_wallets', requireApproval: false }] },
      { serverId: 'GneZ23456789abcdefghj', missing: true, tools: [{ name: 'a', requireApproval: true }, { name: 'b', requireApproval: true }] },
    ] });
    expect(missingServers(config, [yasaku])).toEqual([{ id: 'GneZ23456789abcdefghj', tools: 2 }]);
    expect(missingServers(config, [])).toHaveLength(2);
    expect(withoutGrant(config, 'GneZ23456789abcdefghj').grants.map(g => g.serverId)).toEqual([yasaku.id]);
  });

  it('counts any granted tool, memory or knowledge as private data', () => {
    expect(readsPrivateData(emptyConfig(), emptyRuntime())).toBe(false);
    expect(readsPrivateData(withGrant(emptyConfig(), yasaku, tool('list_wallets'), true), emptyRuntime())).toBe(true);
    expect(readsPrivateData(emptyConfig(), { ...emptyRuntime(), memory: true })).toBe(true);
    expect(readsPrivateData(emptyConfig(), { ...emptyRuntime(), knowledgeIds: ['k'] })).toBe(true);
  });
});

describe('inputs', () => {
  it('reads a dollar cap into cents', () => {
    expect(parseDollars('')).toBe(0);
    expect(parseDollars('$0,50')).toBe(50);
    expect(parseDollars('1.5')).toBe(150);
    expect(parseDollars('abc')).toBeNull();
    expect(parseDollars('1.234')).toBeNull();
  });

  it('parses one domain per line', () => {
    expect(parseDomains(' BCA.co.id\n\nmandiri.co.id \n')).toEqual(['bca.co.id', 'mandiri.co.id']);
  });

  it('keeps only ready sources', () => {
    expect(readySources(sampleKnowledgeSources()).map(s => s.name)).toEqual(['Panduan kategori yasaku']);
  });

  it('finds the chosen provider, model and helper model', () => {
    const chosen = chosenModel(ready({ helperModel: 'google/gemini-2.5-flash' }), providers);
    expect([chosen.provider?.name, chosen.model?.id, chosen.helper?.id]).toEqual(['OpenRouter', 'openai/gpt-4.1-mini', 'google/gemini-2.5-flash']);
    expect(chosenModel({ providerId: 'gone', model: 'x' }, providers)).toEqual({ provider: undefined, model: undefined, helper: undefined });
  });
});

describe('publishBlockers', () => {
  it('lets a ready draft through', () => {
    expect(publishBlockers(ready({ grants: [{ serverId: yasaku.id, missing: false, tools: [] }] }), ctx)).toEqual([]);
  });

  it('asks for a model, names a removed one, and says when the provider was deleted', () => {
    expect(publishBlockers(emptyConfig(), ctx)).toEqual([{ key: 'publish.blocker_model', target: 'model' }]);
    expect(publishBlockers({ ...emptyConfig(), model: 'm', providerMissing: true }, ctx)).toEqual([{ key: 'publish.blocker_provider_missing', target: 'model' }]);
    expect(publishBlockers(ready({ model: 'gone' }), ctx)).toEqual([{ key: 'publish.blocker_model_removed', target: 'model', params: { model: 'gone', provider: 'OpenRouter' } }]);
  });

  it('refuses a cost cap on an unpriced model or helper model', () => {
    const groq = ready({ providerId: 'Pv9pHs4YvT7kMq7WcXe2B', model: 'llama-3.3-70b-versatile', maxCostCents: 20 });
    expect(costCapError(groq, providers)).toBe('limits.unpriced');
    expect(costCapError({ ...groq, maxCostCents: 0 }, providers)).toBe('');
    expect(publishBlockers(groq, ctx)).toEqual([{ key: 'publish.blocker_unpriced', target: 'limits', params: { provider: 'Groq' } }]);
    expect(costCapError(ready({ maxCostCents: 20 }), providers)).toBe('');
  });

  it('names an unpriced helper model on its own field and blocker, not the main model\'s', () => {
    const groqHelper = { ...providers[0]!, models: [...providers[0]!.models, { id: 'free-helper', vision: false }] };
    const config = ready({ helperModel: 'free-helper', maxCostCents: 20 });
    expect(costCapError(config, [groqHelper])).toBe('');
    expect(helperCapError(config, [groqHelper])).toBe('limits.helper_unpriced');
    expect(helperCapError({ ...config, maxCostCents: 0 }, [groqHelper])).toBe('');
    expect(publishBlockers(config, { providers: [groqHelper], servers: [] })).toEqual([{ key: 'publish.blocker_helper_unpriced', target: 'model', field: 'helper', params: { model: 'free-helper', provider: 'OpenRouter' } }]);
  });

  it('needs the granted servers to exist and no old tools', () => {
    const config = ready({ grants: [{ serverId: 'GneZ23456789abcdefghj', missing: true, tools: [] }], tools: [{ name: 'old', requireApproval: false }] });
    expect(publishBlockers(config, ctx).map(b => b.key)).toEqual(['publish.blocker_server_missing', 'publish.blocker_legacy_tools']);
    expect(publishBlockers(ready(), { providers: [], servers: [] })[0]?.key).toBe('publish.blocker_model');
  });
});
