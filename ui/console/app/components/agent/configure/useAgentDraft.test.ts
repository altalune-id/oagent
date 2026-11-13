import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { nextTick } from 'vue';
import { connectError, deferred, setClient, setKnownKeys } from '#test/nuxt-globals';
import { useAgentsStore } from '~/stores/agents';
import { changes, emptyConfig, fromWire, stateOf, toWire } from './agentConfig';
import { autosaveDelayMs, useAgentDraft } from './useAgentDraft';

const usd = (cents: number) => `usd:${cents}`;

const id = 'A7kPq2WxN4mLt8VcR3bZs';
const provider = 'Pv6tQm2ZxR8wKp3VbNc4K';
const server = 'Ms2bVn7QxK4pRt9KwZc3H';
const names = { provider: (p: string) => (p === provider ? 'OpenRouter' : '?'), server: (x: string) => (x === server ? 'yasaku' : x) };
type Wire = ReturnType<typeof toWire> & { providerMissing: boolean, tools: { name: string, requireApproval: boolean }[], mcpServers: { serverId: string, tools: { name: string, requireApproval: boolean }[], missing: boolean }[] };
const liveWire: Wire = { instructions: 'Help.', providerId: provider, model: 'large-2', helperModel: '', temperature: 0.2, maxSteps: 12, maxCostCents: 10, mcpServers: [{ serverId: server, tools: [{ name: 'orders_lookup', requireApproval: false, freeWhen: [] }, { name: 'refunds_create', requireApproval: false, freeWhen: [] }], missing: false }], knowledgeSourceIds: [], knowledgeTuning: { topK: 0, maxTokens: 0 }, memoryStoreId: '', providerMissing: false, tools: [] };
const draftWire: Wire = { ...liveWire, instructions: 'Help. Never promise a date.' };
const live = fromWire(liveWire);
const draft = fromWire(draftWire);
const wireAgent = (config: Wire = draftWire, extra = {}) => ({ id, draft: config, publishedVersion: 12, unpublishedChanges: true, draftUpdatedAt: timestampFromDate(new Date('2026-11-02T01:00:00Z')), ...extra });
const echo = (config: ReturnType<typeof toWire>) => wireAgent({ ...config, providerMissing: false, tools: [], mcpServers: config.mcpServers.map(g => ({ ...g, missing: false })) });

describe('agentConfig', () => {
  it('reads an agent and its draft off the wire, with an empty draft when none is set', () => {
    expect(fromWire()).toEqual(emptyConfig());
    const s = stateOf(wireAgent());
    expect(s.draft).toEqual(draft);
    expect(s.publishedVersion).toBe(12);
    expect(s.draftUpdatedAt?.toISOString()).toBe('2026-11-02T01:00:00.000Z');
    expect(stateOf({ publishedVersion: 0, unpublishedChanges: false }).draftUpdatedAt).toBeUndefined();
  });

  it('reads, sends and lists a change of the memory store, off when the wire leaves it out', () => {
    const storeA = 'Ms4tR7wQ2xKp9VbNc3HzL';
    const storeB = 'Ms8kW3nV6yHp7ZxR5cJtQ';
    const { memoryStoreId: _, ...none } = liveWire;
    expect(fromWire(none as Wire).memoryStoreId).toBe('');
    expect(emptyConfig().memoryStoreId).toBe('');
    expect(fromWire({ ...liveWire, memoryStoreMissing: true }).memoryStoreMissing).toBe(true);
    const on = { ...live, memoryStoreId: storeA };
    expect(fromWire({ ...liveWire, memoryStoreId: storeA }).memoryStoreId).toBe(storeA);
    expect(toWire(on).memoryStoreId).toBe(storeA);
    expect(toWire({ ...on, memoryStoreMissing: true })).not.toHaveProperty('memoryStoreMissing');
    expect(changes(on, live, usd, names)).toEqual([{ key: 'configure.change_memory_on' }]);
    expect(changes(live, on, usd, names)).toEqual([{ key: 'configure.change_memory_off' }]);
    expect(changes({ ...live, memoryStoreId: storeB }, on, usd, names)).toEqual([{ key: 'configure.change_memory_store' }]);
    const stores = { ...names, memoryStore: (s: string) => ({ [storeA]: 'Customers', [storeB]: 'Archive' })[s] };
    expect(changes({ ...live, memoryStoreId: storeB }, on, usd, stores)).toEqual([{ key: 'configure.change_memory_store_named', params: { from: 'Customers', to: 'Archive' } }]);
    expect(changes({ ...live, memoryStoreId: 'Ms2bVn7QxK4pRt9KwZc3H' }, on, usd, stores)).toEqual([{ key: 'configure.change_memory_store' }]);
  });

  it('sends no legacy tools, output-only flags or grants of a deleted server', () => {
    const withLegacy = fromWire({ ...draftWire, providerMissing: true, tools: [{ name: 'old', requireApproval: true }], mcpServers: [...draftWire.mcpServers, { serverId: 'GneZ23456789abcdefghj', tools: [{ name: 'x', requireApproval: true }], missing: true }] });
    expect(withLegacy.tools).toEqual([{ name: 'old', requireApproval: true, freeWhen: [] }]);
    expect(withLegacy.grants.map(g => g.missing)).toEqual([false, true]);
    const sent = toWire(withLegacy);
    expect(sent).not.toHaveProperty('tools');
    expect(sent).not.toHaveProperty('providerMissing');
    expect(sent.mcpServers).toEqual([{ serverId: server, tools: draftWire.mcpServers[0]!.tools.map(t => ({ ...t, freeWhen: [] })) }]);
  });

  it('round-trips a free_when rule, drops it when the tool is not asked, and lists a rule change', () => {
    const rule = [{ arg: 'confirm', values: ['false'], absent: false }];
    const tool = draftWire.mcpServers[0]!.tools[0]!;
    const asked = fromWire({ ...draftWire, mcpServers: [{ ...draftWire.mcpServers[0]!, tools: [{ ...tool, requireApproval: true, freeWhen: rule }] }] });
    expect(asked.grants[0]!.tools[0]!.freeWhen).toEqual(rule);
    expect(toWire(asked).mcpServers[0]!.tools[0]!.freeWhen).toEqual(rule);
    const free = { ...asked, grants: [{ ...asked.grants[0]!, tools: [{ ...asked.grants[0]!.tools[0]!, requireApproval: false }] }] };
    expect(toWire(free).mcpServers[0]!.tools[0]!.freeWhen).toEqual([]);
    const stricter = { ...asked, grants: [{ ...asked.grants[0]!, tools: [{ ...asked.grants[0]!.tools[0]!, freeWhen: [] }] }] };
    expect(changes(stricter, asked, usd, names).map(c => c.key)).toContain('configure.change_tool_rule');
  });

  it('lists every change against the live version, and a first publish when there is none', () => {
    expect(changes(draft, undefined, usd, names)).toEqual([{ key: 'configure.change_first' }]);
    expect(changes(live, live, usd, names)).toEqual([]);
    const next = {
      ...live,
      instructions: 'x',
      model: 'small-3',
      helperModel: 'tiny-1',
      temperature: 0.4,
      maxSteps: 8,
      maxCostCents: 25,
      grants: [{ serverId: server, missing: false, tools: [{ name: 'refunds_create', requireApproval: true }, { name: 'search', requireApproval: false }, { name: 'audit', requireApproval: true }] }],
    };
    expect(changes(next, live, usd, names)).toEqual([
      { key: 'configure.change_instructions' },
      { key: 'configure.change_provider_model', params: { provider: 'OpenRouter', model: 'small-3' } },
      { key: 'configure.change_helper', params: { model: 'tiny-1' } },
      { key: 'configure.change_temperature', params: { from: 0.2, to: 0.4 } },
      { key: 'configure.change_max_steps', params: { from: 12, to: 8 } },
      { key: 'configure.change_budget', params: { from: 'usd:10', to: 'usd:25' } },
      { key: 'configure.change_tool_needs_approval', params: { tool: 'yasaku__refunds_create' } },
      { key: 'configure.change_tool_added', params: { tool: 'yasaku__search' } },
      { key: 'configure.change_tool_added_approval', params: { tool: 'yasaku__audit' } },
      { key: 'configure.change_tool_removed', params: { tool: 'yasaku__orders_lookup' } },
    ]);
    expect(changes({ ...live, providerId: '', model: '', helperModel: '' }, { ...live, helperModel: 'tiny-1' }, usd, names).slice(0, 2)).toEqual([
      { key: 'configure.change_provider_model', params: { provider: '—', model: '—' } },
      { key: 'configure.change_helper_same' },
    ]);
    const relaxed = { ...live, grants: [{ ...live.grants[0]!, tools: [{ name: 'orders_lookup', requireApproval: false }, { name: 'refunds_create', requireApproval: false }] }] };
    const strict = { ...live, grants: [{ ...live.grants[0]!, tools: [{ name: 'orders_lookup', requireApproval: true }, { name: 'refunds_create', requireApproval: false }] }] };
    expect(changes(relaxed, strict, usd, names)).toEqual([{ key: 'configure.change_tool_no_approval', params: { tool: 'yasaku__orders_lookup' } }]);
  });
});

describe('agentConfig: knowledge sources', () => {
  it('reads, sends and lists a change of the bound knowledge sources, in any order', () => {
    const bound = fromWire({ ...liveWire, knowledgeSourceIds: ['Ks1', 'Ks2'] });
    expect(bound.knowledgeSourceIds).toEqual(['Ks1', 'Ks2']);
    expect(fromWire(liveWire).knowledgeSourceIds).toEqual([]);
    expect(toWire(bound).knowledgeSourceIds).toEqual(['Ks1', 'Ks2']);
    expect(changes({ ...bound, knowledgeSourceIds: ['Ks2', 'Ks1'] }, bound, usd, names)).toEqual([]);
    expect(changes({ ...bound, knowledgeSourceIds: ['Ks2'] }, bound, usd, names)).toEqual([{ key: 'configure.change_knowledge' }]);
    const tuned = fromWire({ ...liveWire, knowledgeTuning: { topK: 3, minScore: 0, maxTokens: 0 } });
    expect([tuned.knowledgeTuning, fromWire(liveWire).knowledgeTuning]).toEqual([{ topK: 3, minScore: 0, maxTokens: 0 }, { topK: 0, maxTokens: 0 }]);
    expect(toWire(tuned).knowledgeTuning).toEqual({ topK: 3, minScore: 0, maxTokens: 0 });
    expect(toWire(fromWire(liveWire)).knowledgeTuning, 'an unset minimum relevance stays unset').toEqual({ topK: 0, maxTokens: 0 });
    expect(changes(tuned, fromWire(liveWire), usd, names)).toEqual([{ key: 'configure.change_knowledge_tuning' }]);
    expect(changes(fromWire({ ...liveWire, knowledgeTuning: { topK: 0, maxTokens: 0 } }), fromWire(liveWire), usd, names)).toEqual([]);
    const gone = fromWire({ ...liveWire, knowledgeSourcesMissing: 2 });
    expect([gone.knowledgeSourcesMissing, fromWire(liveWire).knowledgeSourcesMissing]).toEqual([2, 0]);
    expect(toWire(gone)).not.toHaveProperty('knowledgeSourcesMissing');
  });
});

describe('useAgentDraft', () => {
  let client: Record<string, ReturnType<typeof vi.fn>>;

  beforeEach(() => {
    setActivePinia(createPinia());
    vi.useFakeTimers();
    client = {
      getAgent: vi.fn().mockResolvedValue({ agent: wireAgent() }),
      listAgentVersions: vi.fn().mockResolvedValue({ versions: [{ version: 12, config: liveWire }] }),
      updateAgentDraft: vi.fn().mockImplementation(async ({ config }) => ({ agent: echo(config) })),
      publishAgent: vi.fn().mockResolvedValue({ agent: wireAgent(draftWire, { publishedVersion: 13, unpublishedChanges: false }), version: { version: 13, config: draftWire }, created: true }),
    };
    setClient('$agentClient', client);
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('loads the draft and the live version, without saving what it loaded', async () => {
    const d = useAgentDraft(id, names);
    await d.load();
    await vi.advanceTimersByTimeAsync(autosaveDelayMs);
    expect(client.getAgent).toHaveBeenCalledWith({ id });
    expect(client.listAgentVersions).toHaveBeenCalledWith({ agentId: id, limit: 1 });
    expect(d.config.value).toEqual(draft);
    expect(d.live.value).toEqual(live);
    expect(d.diff.value).toEqual([{ key: 'configure.change_instructions' }]);
    expect(client.updateAgentDraft).not.toHaveBeenCalled();
    expect(d.dirty.value).toBe(false);
  });

  it('treats an agent with no versions as a first publish', async () => {
    client.listAgentVersions!.mockResolvedValue({ versions: [] });
    const d = useAgentDraft(id, names);
    await d.load();
    expect(d.live.value).toBeUndefined();
    expect(d.diff.value).toEqual([{ key: 'configure.change_first' }]);
  });

  it('autosaves an edit once, a second after the last keystroke', async () => {
    const d = useAgentDraft(id, names);
    await d.load();
    d.config.value = { ...d.config.value, model: 'small-3' };
    await vi.advanceTimersByTimeAsync(autosaveDelayMs / 2);
    d.config.value = { ...d.config.value, temperature: 0.5 };
    await vi.advanceTimersByTimeAsync(autosaveDelayMs / 2);
    expect(d.dirty.value).toBe(true);
    expect(client.updateAgentDraft).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(autosaveDelayMs);
    expect(client.updateAgentDraft).toHaveBeenCalledOnce();
    expect(client.updateAgentDraft!.mock.calls[0]![0]).toEqual({ id, config: toWire({ ...draft, model: 'small-3', temperature: 0.5 }) });
    expect(d.dirty.value).toBe(false);
    expect(await d.save()).toBe(true);
    expect(client.updateAgentDraft).toHaveBeenCalledOnce();
  });

  it('keeps the edit and shows the refusal when saving fails', async () => {
    setKnownKeys('error.save_draft');
    client.updateAgentDraft!.mockRejectedValue(connectError('GEN004'));
    const d = useAgentDraft(id, names);
    await d.load();
    d.config.value = { ...d.config.value, maxSteps: 500 };
    await vi.advanceTimersByTimeAsync(autosaveDelayMs);
    expect(d.saveError.value).not.toBeNull();
    expect(d.config.value.maxSteps).toBe(500);
    expect(d.dirty.value).toBe(true);
    expect(await d.publish()).toBeNull();
    expect(client.publishAgent).not.toHaveBeenCalled();
  });

  it('saves pending edits before publishing, then takes the new live version', async () => {
    const nextWire = { ...draftWire, model: 'small-3' };
    const next = fromWire(nextWire);
    client.publishAgent!.mockResolvedValue({ agent: wireAgent(nextWire, { publishedVersion: 13, unpublishedChanges: false }), version: { version: 13, config: nextWire }, created: true });
    const d = useAgentDraft(id, names);
    await d.load();
    d.config.value = { ...d.config.value, model: 'small-3' };
    const result = await d.publish();
    expect(client.updateAgentDraft).toHaveBeenCalledOnce();
    expect(client.publishAgent).toHaveBeenCalledWith({ id });
    expect(result).toEqual({ created: true, version: 13 });
    expect(d.state.value?.publishedVersion).toBe(13);
    expect(d.live.value).toEqual(next);
    expect(d.diff.value).toEqual([]);
    expect(d.dirty.value).toBe(false);
  });

  it('reports a failed load and never publishes after it', async () => {
    client.getAgent!.mockRejectedValue(connectError('NOT_FOUND'));
    const d = useAgentDraft(id, names);
    await d.load();
    expect(d.loadError.value).not.toBeNull();
    expect(await d.publish()).toBeNull();
    expect(client.publishAgent).not.toHaveBeenCalled();
    expect(d.publishing.value).toBe(false);
  });

  it('never autosaves after a failed load, so an empty config never overwrites the draft', async () => {
    client.listAgentVersions!.mockRejectedValue(connectError('GEN900'));
    const d = useAgentDraft(id, names);
    await d.load();
    d.config.value = { ...d.config.value, model: 'small-3' };
    await vi.advanceTimersByTimeAsync(autosaveDelayMs);
    expect(await d.save()).toBe(false);
    expect(client.updateAgentDraft).not.toHaveBeenCalled();
  });

  it('does not retry a failed autosave on its own, retries on demand, and clears the error when the edit is reverted', async () => {
    const d = useAgentDraft(id, names);
    await d.load();
    client.updateAgentDraft!.mockRejectedValueOnce(new Error('offline'));
    d.config.value = { ...d.config.value, model: 'small-3' };
    await vi.advanceTimersByTimeAsync(autosaveDelayMs * 5);
    expect(client.updateAgentDraft).toHaveBeenCalledOnce();
    expect(d.saveError.value).toEqual({ message: 'error.save_draft' });
    expect(await d.save()).toBe(true);
    expect(client.updateAgentDraft).toHaveBeenCalledTimes(2);
    expect(d.saveError.value).toBeNull();
    client.updateAgentDraft!.mockRejectedValueOnce(new Error('offline'));
    d.config.value = { ...d.config.value, model: 'tiny-1' };
    await vi.advanceTimersByTimeAsync(autosaveDelayMs);
    expect(d.saveError.value).not.toBeNull();
    d.config.value = { ...d.config.value, model: 'small-3' };
    await vi.advanceTimersByTimeAsync(0);
    expect(d.saveError.value).toBeNull();
    expect(d.dirty.value).toBe(false);
  });

  it('drops the old tools once a save comes back without them', async () => {
    client.getAgent!.mockResolvedValue({ agent: wireAgent({ ...draftWire, tools: [{ name: 'old', requireApproval: false }] }) });
    const d = useAgentDraft(id, names);
    await d.load();
    expect(d.config.value.tools).toHaveLength(1);
    d.config.value = { ...d.config.value, model: 'small-3' };
    await vi.advanceTimersByTimeAsync(autosaveDelayMs);
    expect(d.config.value.tools).toEqual([]);
    expect(d.dirty.value).toBe(false);
  });

  it('places an AGT007 refusal under its field and stops autosaving it, then clears it on the next save', async () => {
    setKnownKeys('apperror.AGT007_model', 'apperror.AGT006');
    const d = useAgentDraft(id, names);
    await d.load();
    client.updateAgentDraft!.mockRejectedValueOnce(connectError('AGT007', { reason: 'model' }));
    d.config.value = { ...d.config.value, model: 'gone' };
    await vi.advanceTimersByTimeAsync(autosaveDelayMs * 3);
    expect(client.updateAgentDraft).toHaveBeenCalledOnce();
    expect(d.fieldError.value).toEqual({ reason: 'model', message: 'apperror.AGT007_model' });
    d.config.value = { ...d.config.value, model: 'large-2' };
    await vi.advanceTimersByTimeAsync(autosaveDelayMs);
    expect(d.fieldError.value).toBeNull();
    client.publishAgent!.mockRejectedValueOnce(connectError('AGT006'));
    expect(await d.publish()).toBeNull();
    expect(d.publishError.value?.message).toBe('apperror.AGT006');
    expect(d.fieldError.value).toBeNull();
    client.publishAgent!.mockRejectedValueOnce(connectError('AGT007', { reason: 'budget_unpriced' }));
    await d.publish();
    expect(d.fieldError.value?.reason).toBe('budget_unpriced');
  });

  it('keeps the refused tool name with an AGT007 publish refusal', async () => {
    setKnownKeys('apperror.AGT007_unapproved_tool');
    const d = useAgentDraft(id, names);
    await d.load();
    client.publishAgent!.mockRejectedValueOnce(connectError('AGT007', { reason: 'unapproved_tool', name: 'yasaku__refunds_create' }));
    expect(await d.publish()).toBeNull();
    expect(d.fieldError.value).toEqual({ reason: 'unapproved_tool', name: 'yasaku__refunds_create', message: 'apperror.AGT007_unapproved_tool|{"name":"yasaku__refunds_create"}' });
  });

  it('drops a publish refusal as soon as the draft changes', async () => {
    setKnownKeys('apperror.AGT007_unapproved_tool');
    const d = useAgentDraft(id, names);
    await d.load();
    client.publishAgent!.mockRejectedValueOnce(connectError('AGT007', { reason: 'unapproved_tool', name: 'yasaku__refunds_create' }));
    await d.publish();
    expect(d.fieldError.value?.name).toBe('yasaku__refunds_create');
    d.config.value = { ...d.config.value, instructions: 'Changed.' };
    await nextTick();
    expect(d.fieldError.value).toBeNull();
    expect(d.publishError.value).toBeNull();
  });

  it('never puts an AGT007 tool_name refusal under a tool row', async () => {
    const d = useAgentDraft(id, names);
    await d.load();
    client.publishAgent!.mockRejectedValueOnce(connectError('AGT007', { reason: 'tool_name', name: 'yasaku__refunds_create' }));
    await d.publish();
    expect(d.fieldError.value?.reason).toBe('tool_name');
    expect(d.fieldError.value?.name).toBeUndefined();
  });

  it('reports a failed publish', async () => {
    const d = useAgentDraft(id, names);
    await d.load();
    client.publishAgent!.mockRejectedValue(connectError('GEN004'));
    expect(await d.publish()).toBeNull();
    expect(d.publishError.value).not.toBeNull();
    expect(d.publishing.value).toBe(false);
  });

  it('publishes once when Publish is pressed twice, and shows a failed pre-publish save in the publish error', async () => {
    const d = useAgentDraft(id, names);
    await d.load();
    d.config.value = { ...d.config.value, model: 'small-3' };
    const both = await Promise.all([d.publish(), d.publish()]);
    expect(client.publishAgent).toHaveBeenCalledOnce();
    expect(both[1]).toBeNull();
    d.config.value = { ...d.config.value, model: 'tiny-1' };
    client.updateAgentDraft!.mockRejectedValueOnce(connectError('GEN900'));
    expect(await d.publish()).toBeNull();
    expect(d.publishError.value).not.toBeNull();
    expect(d.publishError.value).toBe(d.saveError.value);
  });

  it('warns before closing the tab with unsaved edits, and blocks leaving when the save fails', async () => {
    const d = useAgentDraft(id, names);
    await d.load();
    const quiet = new Event('beforeunload', { cancelable: true }) as BeforeUnloadEvent;
    d.warnUnload(quiet);
    expect(quiet.defaultPrevented).toBe(false);
    expect(await d.leaveGuard()).toBe(true);
    d.config.value = { ...d.config.value, model: 'small-3' };
    await vi.advanceTimersByTimeAsync(0);
    const closing = new Event('beforeunload', { cancelable: true }) as BeforeUnloadEvent;
    d.warnUnload(closing);
    expect(closing.defaultPrevented).toBe(true);
    client.updateAgentDraft!.mockRejectedValueOnce(connectError('GEN900'));
    expect(await d.leaveGuard()).toBe(false);
    expect(await d.leaveGuard()).toBe(true);
  });

  it('lets the second leave through while the save keeps failing', async () => {
    const d = useAgentDraft(id, names);
    await d.load();
    d.config.value = { ...d.config.value, model: 'small-3' };
    await vi.advanceTimersByTimeAsync(0);
    client.updateAgentDraft!.mockRejectedValue(connectError('GEN900'));
    expect(await d.leaveGuard()).toBe(false);
    expect(await d.leaveGuard()).toBe(true);
  });

  it('blocks a leave again after a save succeeds between two failures', async () => {
    const d = useAgentDraft(id, names);
    await d.load();
    d.config.value = { ...d.config.value, model: 'small-3' };
    await vi.advanceTimersByTimeAsync(0);
    client.updateAgentDraft!.mockRejectedValueOnce(connectError('GEN900'));
    expect(await d.leaveGuard()).toBe(false);
    expect(await d.save()).toBe(true);
    d.config.value = { ...d.config.value, model: 'tiny-1' };
    await vi.advanceTimersByTimeAsync(0);
    client.updateAgentDraft!.mockRejectedValueOnce(connectError('GEN900'));
    expect(await d.leaveGuard()).toBe(false);
  });

  it('keeps edits typed while a publish is in flight, and saves them after', async () => {
    const d = useAgentDraft(id, names);
    await d.load();
    const pending = deferred<unknown>();
    client.publishAgent!.mockImplementationOnce(async () => {
      await pending.promise;
      return { agent: wireAgent(draftWire, { publishedVersion: 13, unpublishedChanges: false }), version: { version: 13, config: draftWire }, created: true };
    });
    const run = d.publish();
    await vi.advanceTimersByTimeAsync(0);
    d.config.value = { ...d.config.value, model: 'small-3' };
    pending.resolve(undefined);
    await run;
    expect(d.config.value.model).toBe('small-3');
    expect(d.live.value).toEqual(draft);
    expect(d.dirty.value).toBe(true);
    await vi.advanceTimersByTimeAsync(autosaveDelayMs);
    expect(client.updateAgentDraft!.mock.calls.at(-1)![0]).toEqual({ id, config: toWire({ ...draft, model: 'small-3' }) });
  });

  it('runs one save at a time and saves the newest edit after the one in flight', async () => {
    const d = useAgentDraft(id, names);
    await d.load();
    const first = deferred<unknown>();
    client.updateAgentDraft!.mockImplementationOnce(async ({ config }: { config: ReturnType<typeof toWire> }) => {
      await first.promise;
      return { agent: echo(config) };
    });
    d.config.value = { ...d.config.value, model: 'small-3' };
    await vi.advanceTimersByTimeAsync(autosaveDelayMs);
    expect(client.updateAgentDraft).toHaveBeenCalledOnce();
    d.config.value = { ...d.config.value, temperature: 0.9 };
    await vi.advanceTimersByTimeAsync(autosaveDelayMs);
    expect(client.updateAgentDraft).toHaveBeenCalledOnce();
    first.resolve(undefined);
    await vi.advanceTimersByTimeAsync(autosaveDelayMs);
    expect(client.updateAgentDraft).toHaveBeenCalledTimes(2);
    expect(client.updateAgentDraft!.mock.calls[1]![0]).toEqual({ id, config: toWire({ ...draft, model: 'small-3', temperature: 0.9 }) });
    expect(d.dirty.value).toBe(false);
  });

  it('updates the agents list after a save and a publish, without a reload', async () => {
    const agents = useAgentsStore();
    agents.items = [{ id, projectId: 'P1', slug: 'support', name: 'Support', description: '', publishedVersion: 12, unpublishedChanges: false }];
    const d = useAgentDraft(id, names);
    await d.load();
    d.config.value = { ...d.config.value, model: 'small-3' };
    await vi.advanceTimersByTimeAsync(autosaveDelayMs);
    expect(agents.items[0]).toMatchObject({ publishedVersion: 12, unpublishedChanges: true });
    await d.publish();
    expect(agents.items[0]).toMatchObject({ publishedVersion: 13, unpublishedChanges: false });
  });

  it('copes with empty responses', async () => {
    client.getAgent!.mockResolvedValue({});
    client.listAgentVersions!.mockResolvedValue({});
    client.updateAgentDraft!.mockResolvedValue({});
    client.publishAgent!.mockResolvedValue({ created: false });
    const d = useAgentDraft(id, names);
    await d.load();
    expect(d.state.value).toBeNull();
    d.config.value = { ...d.config.value, model: 'x' };
    expect(await d.save()).toBe(false);
    expect(await d.publish()).toBeNull();
    expect(client.updateAgentDraft).not.toHaveBeenCalled();
  });
});
