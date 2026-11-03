import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { connectError, deferred, setClient, setKnownKeys } from '#test/nuxt-globals';
import { useAgentsStore } from '~/stores/agents';
import { changes, emptyConfig, fromWire, stateOf } from './agentConfig';
import { autosaveDelayMs, useAgentDraft } from './useAgentDraft';

const id = 'A7kPq2WxN4mLt8VcR3bZs';
const live = { instructions: 'Help.', model: 'large-2', temperature: 0.2, maxSteps: 12, maxCostCents: 10, tools: [{ name: 'orders.lookup', requireApproval: false }, { name: 'billing.refunds.create', requireApproval: false }] };
const draft = { ...live, instructions: 'Help. Never promise a date.' };
const wireAgent = (config = draft, extra = {}) => ({ id, draft: config, publishedVersion: 12, unpublishedChanges: true, draftUpdatedAt: timestampFromDate(new Date('2026-11-02T01:00:00Z')), ...extra });

describe('agentConfig', () => {
  it('reads an agent and its draft off the wire, with an empty draft when none is set', () => {
    expect(fromWire()).toEqual(emptyConfig());
    const s = stateOf(wireAgent());
    expect(s.draft).toEqual(draft);
    expect(s.publishedVersion).toBe(12);
    expect(s.draftUpdatedAt?.toISOString()).toBe('2026-11-02T01:00:00.000Z');
    expect(stateOf({ publishedVersion: 0, unpublishedChanges: false }).draftUpdatedAt).toBeUndefined();
  });

  it('lists every change against the live version, and a first publish when there is none', () => {
    expect(changes(draft)).toEqual([{ key: 'configure.change_first' }]);
    expect(changes(live, live)).toEqual([]);
    const next = {
      instructions: 'x',
      model: 'small-3',
      temperature: 0.4,
      maxSteps: 8,
      maxCostCents: 25,
      tools: [{ name: 'billing.refunds.create', requireApproval: true }, { name: 'knowledge.search', requireApproval: false }, { name: 'audit.write', requireApproval: true }],
    };
    expect(changes(next, live)).toEqual([
      { key: 'configure.change_instructions' },
      { key: 'configure.change_model', params: { from: 'large-2', to: 'small-3' } },
      { key: 'configure.change_temperature', params: { from: 0.2, to: 0.4 } },
      { key: 'configure.change_max_steps', params: { from: 12, to: 8 } },
      { key: 'configure.change_budget', params: { from: 0.1, to: 0.25 } },
      { key: 'configure.change_tool_needs_approval', params: { tool: 'billing.refunds.create' } },
      { key: 'configure.change_tool_added', params: { tool: 'knowledge.search' } },
      { key: 'configure.change_tool_added_approval', params: { tool: 'audit.write' } },
      { key: 'configure.change_tool_removed', params: { tool: 'orders.lookup' } },
    ]);
    expect(changes({ ...live, model: '', tools: [{ name: 'orders.lookup', requireApproval: true }] }, { ...live, tools: [{ name: 'orders.lookup', requireApproval: true }] })[0]).toEqual({ key: 'configure.change_model', params: { from: 'large-2', to: '—' } });
    expect(changes(live, { ...live, model: '', tools: [{ name: 'orders.lookup', requireApproval: true }, { name: 'billing.refunds.create', requireApproval: false }] })).toEqual([
      { key: 'configure.change_model', params: { from: '—', to: 'large-2' } },
      { key: 'configure.change_tool_no_approval', params: { tool: 'orders.lookup' } },
    ]);
  });
});

describe('useAgentDraft', () => {
  let client: Record<string, ReturnType<typeof vi.fn>>;

  beforeEach(() => {
    setActivePinia(createPinia());
    vi.useFakeTimers();
    client = {
      getAgent: vi.fn().mockResolvedValue({ agent: wireAgent() }),
      listAgentVersions: vi.fn().mockResolvedValue({ versions: [{ version: 12, config: live }] }),
      updateAgentDraft: vi.fn().mockImplementation(async ({ config }) => ({ agent: wireAgent(config) })),
      publishAgent: vi.fn().mockResolvedValue({ agent: wireAgent(draft, { publishedVersion: 13, unpublishedChanges: false }), version: { version: 13, config: draft }, created: true }),
    };
    setClient('$agentClient', client);
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('loads the draft and the live version, without saving what it loaded', async () => {
    const d = useAgentDraft(id);
    await d.load();
    await vi.advanceTimersByTimeAsync(autosaveDelayMs);
    expect(client.getAgent).toHaveBeenCalledWith({ id });
    expect(client.listAgentVersions).toHaveBeenCalledWith({ agentId: id });
    expect(d.config.value).toEqual(draft);
    expect(d.live.value).toEqual(live);
    expect(d.diff.value).toEqual([{ key: 'configure.change_instructions' }]);
    expect(client.updateAgentDraft).not.toHaveBeenCalled();
    expect(d.dirty.value).toBe(false);
  });

  it('treats an agent with no versions as a first publish', async () => {
    client.listAgentVersions!.mockResolvedValue({ versions: [] });
    const d = useAgentDraft(id);
    await d.load();
    expect(d.live.value).toBeUndefined();
    expect(d.diff.value).toEqual([{ key: 'configure.change_first' }]);
  });

  it('autosaves an edit once, a second after the last keystroke', async () => {
    const d = useAgentDraft(id);
    await d.load();
    d.config.value = { ...d.config.value, model: 'small-3' };
    await vi.advanceTimersByTimeAsync(autosaveDelayMs / 2);
    d.config.value = { ...d.config.value, temperature: 0.5 };
    await vi.advanceTimersByTimeAsync(autosaveDelayMs / 2);
    expect(d.dirty.value).toBe(true);
    expect(client.updateAgentDraft).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(autosaveDelayMs);
    expect(client.updateAgentDraft).toHaveBeenCalledOnce();
    expect(client.updateAgentDraft!.mock.calls[0]![0]).toEqual({ id, config: { ...draft, model: 'small-3', temperature: 0.5 } });
    expect(d.dirty.value).toBe(false);
    expect(await d.save()).toBe(true);
    expect(client.updateAgentDraft).toHaveBeenCalledOnce();
  });

  it('keeps the edit and shows the refusal when saving fails', async () => {
    setKnownKeys('error.save_draft');
    client.updateAgentDraft!.mockRejectedValue(connectError('GEN004'));
    const d = useAgentDraft(id);
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
    const next = { ...draft, model: 'small-3' };
    client.publishAgent!.mockResolvedValue({ agent: wireAgent(next, { publishedVersion: 13, unpublishedChanges: false }), version: { version: 13, config: next }, created: true });
    const d = useAgentDraft(id);
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
    const d = useAgentDraft(id);
    await d.load();
    expect(d.loadError.value).not.toBeNull();
    expect(await d.publish()).toBeNull();
    expect(client.publishAgent).not.toHaveBeenCalled();
    expect(d.publishing.value).toBe(false);
  });

  it('never autosaves after a failed load, so an empty config never overwrites the draft', async () => {
    client.listAgentVersions!.mockRejectedValue(connectError('GEN900'));
    const d = useAgentDraft(id);
    await d.load();
    d.config.value = { ...d.config.value, model: 'small-3' };
    await vi.advanceTimersByTimeAsync(autosaveDelayMs);
    expect(await d.save()).toBe(false);
    expect(client.updateAgentDraft).not.toHaveBeenCalled();
  });

  it('reports a failed publish', async () => {
    const d = useAgentDraft(id);
    await d.load();
    client.publishAgent!.mockRejectedValue(connectError('GEN004'));
    expect(await d.publish()).toBeNull();
    expect(d.publishError.value).not.toBeNull();
    expect(d.publishing.value).toBe(false);
  });

  it('publishes once when Publish is pressed twice, and shows a failed pre-publish save in the publish error', async () => {
    const d = useAgentDraft(id);
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
    const d = useAgentDraft(id);
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
    const d = useAgentDraft(id);
    await d.load();
    d.config.value = { ...d.config.value, model: 'small-3' };
    await vi.advanceTimersByTimeAsync(0);
    client.updateAgentDraft!.mockRejectedValue(connectError('GEN900'));
    expect(await d.leaveGuard()).toBe(false);
    expect(await d.leaveGuard()).toBe(true);
  });

  it('keeps edits typed while a publish is in flight, and saves them after', async () => {
    const d = useAgentDraft(id);
    await d.load();
    const pending = deferred<unknown>();
    client.publishAgent!.mockImplementationOnce(async () => {
      await pending.promise;
      return { agent: wireAgent(draft, { publishedVersion: 13, unpublishedChanges: false }), version: { version: 13, config: draft }, created: true };
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
    expect(client.updateAgentDraft!.mock.calls.at(-1)![0]).toEqual({ id, config: { ...draft, model: 'small-3' } });
  });

  it('runs one save at a time and saves the newest edit after the one in flight', async () => {
    const d = useAgentDraft(id);
    await d.load();
    const first = deferred<unknown>();
    client.updateAgentDraft!.mockImplementationOnce(async ({ config }: { config: typeof draft }) => {
      await first.promise;
      return { agent: wireAgent(config) };
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
    expect(client.updateAgentDraft!.mock.calls[1]![0]).toEqual({ id, config: { ...draft, model: 'small-3', temperature: 0.9 } });
    expect(d.dirty.value).toBe(false);
  });

  it('updates the agents list after a save and a publish, without a reload', async () => {
    const agents = useAgentsStore();
    agents.items = [{ id, projectId: 'P1', slug: 'support', name: 'Support', description: '', publishedVersion: 12, unpublishedChanges: false }];
    const d = useAgentDraft(id);
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
    const d = useAgentDraft(id);
    await d.load();
    expect(d.state.value).toBeNull();
    d.config.value = { ...d.config.value, model: 'x' };
    expect(await d.save()).toBe(false);
    expect(await d.publish()).toBeNull();
    expect(client.updateAgentDraft).not.toHaveBeenCalled();
  });
});
