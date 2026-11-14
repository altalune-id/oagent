import type { ModelProvider, ProviderInput } from './modelProviders';
import { create } from '@bufbuild/protobuf';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { errorCode } from '#base/lib/apperror/message';
import { ModelProviderKind, ModelProviderSchema, ModelSchema } from '#gen/modelprovider/v1/modelprovider_pb';
import { connectError, deferred, setClient, setKnownKeys } from '#test/nuxt-globals';
import { createRequest, toModelProvider, updateRequest, useModelProvidersStore } from './modelProviders';

const orgId = 'V3StGXR8Z5jdHj6BmyT2a';
const projectId = 'x6MH9WPWjktMwCBKHy4pF';
const prices = { inputMicros: 400_000n, outputMicros: 1_600_000n, cacheReadMicros: 100_000n, cacheWriteMicros: 0n };
const providerPB = create(ModelProviderSchema, { id: 'UxVy4UK3vmrHzPZ273hCA', orgId, projectId, name: 'OpenRouter', kind: ModelProviderKind.OPENAI_COMPATIBLE, baseUrl: 'https://openrouter.ai/api/v1', apiKeySet: true, models: [{ id: 'openai/gpt-4.1-mini', vision: true, prices }, { id: 'llama', vision: false }, { id: 'text-embedding-3-small', vision: false, embedding: true, prices }] });
const provider: ModelProvider = { id: 'UxVy4UK3vmrHzPZ273hCA', orgId, projectId, name: 'OpenRouter', kind: ModelProviderKind.OPENAI_COMPATIBLE, baseUrl: 'https://openrouter.ai/api/v1', keySet: true, models: [{ id: 'openai/gpt-4.1-mini', vision: true, embedding: false, prices }, { id: 'llama', vision: false, embedding: false, prices: undefined }, { id: 'text-embedding-3-small', vision: false, embedding: true, prices }] };
const input: ProviderInput = { name: ' OpenRouter ', kind: ModelProviderKind.OPENAI_COMPATIBLE, baseUrl: ' https://openrouter.ai/api/v1 ', apiKey: '  sk-or-1  ', models: [{ id: ' openai/gpt-4.1-mini ', vision: true, embedding: false, prices }, { id: 'text-embedding-3-small', vision: false, embedding: true, prices }] };

describe('model provider mapping', () => {
  it('copies the fields it needs, keeping prices in micro-USD and never a key', () => {
    expect(toModelProvider(providerPB)).toEqual(provider);
  });

  it('trims the request and sends a key only when one is typed', () => {
    const add = createRequest(orgId, projectId, input);
    expect(add).toMatchObject({ orgId, projectId, name: 'OpenRouter', kind: ModelProviderKind.OPENAI_COMPATIBLE, baseUrl: 'https://openrouter.ai/api/v1', apiKey: 'sk-or-1' });
    expect(add.models[0]).toMatchObject({ id: 'openai/gpt-4.1-mini', embedding: false, prices });
    expect(add.models[1]).toMatchObject({ id: 'text-embedding-3-small', embedding: true });
    expect(updateRequest(provider.id, input).models[1]!.embedding).toBe(true);
    expect(updateRequest(provider.id, { ...input, apiKey: '   ' }).apiKey).toBeUndefined();
    expect(updateRequest(provider.id, input).apiKey).toBe('sk-or-1');
  });

  it('reads the kind and sends the chosen kind on create and update', () => {
    expect(toModelProvider(create(ModelProviderSchema, { ...providerPB, kind: ModelProviderKind.ANTHROPIC })).kind).toBe(ModelProviderKind.ANTHROPIC);
    const anthropic = { ...input, kind: ModelProviderKind.ANTHROPIC };
    expect(createRequest(orgId, projectId, anthropic).kind).toBe(ModelProviderKind.ANTHROPIC);
    expect(updateRequest(provider.id, anthropic).kind).toBe(ModelProviderKind.ANTHROPIC);
    expect(updateRequest(provider.id, input).kind).toBe(ModelProviderKind.OPENAI_COMPATIBLE);
  });

  it('keeps a transcription model and its audio price through a read and a save', () => {
    const audio = { inputMicros: 0n, outputMicros: 0n, cacheReadMicros: 0n, cacheWriteMicros: 0n, audioMinuteMicros: 6_000n };
    const read = toModelProvider(create(ModelProviderSchema, { ...providerPB, models: [create(ModelSchema, { id: 'whisper-1', transcription: true, prices: audio })] }));
    expect(read.models[0]).toEqual({ id: 'whisper-1', vision: false, embedding: false, transcription: true, prices: audio });
    const sent = updateRequest(provider.id, { ...input, models: read.models });
    expect(sent.models[0]).toMatchObject({ id: 'whisper-1', transcription: true, prices: { audioMinuteMicros: 6_000n } });
    expect(createRequest(orgId, projectId, { ...input, models: read.models }).models[0]!.transcription).toBe(true);
  });

  it('never sends the embedding flag for an Anthropic provider, which has no embeddings API', () => {
    const anthropic = { ...input, kind: ModelProviderKind.ANTHROPIC };
    expect(createRequest(orgId, projectId, anthropic).models.map(m => m.embedding)).toEqual([false, false]);
    expect(updateRequest(provider.id, anthropic).models.map(m => m.embedding)).toEqual([false, false]);
  });
});

describe('useModelProvidersStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('lists the providers of a project', async () => {
    const listModelProviders = vi.fn().mockResolvedValue({ modelProviders: [providerPB] });
    setClient('$modelProviderClient', { listModelProviders });
    const store = useModelProvidersStore();
    await store.load(projectId);
    expect(listModelProviders).toHaveBeenCalledWith({ projectId });
    expect(store.items).toEqual([provider]);
  });

  it('treats a missing list as empty and shows a load error', async () => {
    setClient('$modelProviderClient', { listModelProviders: vi.fn().mockResolvedValue({}) });
    const store = useModelProvidersStore();
    await store.load(projectId);
    expect(store.items).toEqual([]);
    setKnownKeys('apperror.GEN900');
    setClient('$modelProviderClient', { listModelProviders: vi.fn().mockRejectedValue(connectError('GEN900')) });
    setActivePinia(createPinia());
    const failing = useModelProvidersStore();
    await failing.load(projectId);
    expect(failing.error).toEqual({ message: 'apperror.GEN900', reference: 'error.ref|{"id":"req-1"}' });
    expect(failing.loading).toBe(false);
  });

  it('shares an in-flight load, skips a loaded project and drops a stale answer', async () => {
    const first = deferred<{ modelProviders: typeof providerPB[] }>();
    const listModelProviders = vi.fn().mockReturnValueOnce(first.promise).mockResolvedValue({ modelProviders: [] });
    setClient('$modelProviderClient', { listModelProviders });
    const store = useModelProvidersStore();
    await store.ensureLoaded('');
    const a = store.ensureLoaded(projectId);
    const b = store.ensureLoaded(projectId);
    expect(listModelProviders).toHaveBeenCalledTimes(1);
    await store.load(projectId);
    first.resolve({ modelProviders: [providerPB] });
    await Promise.all([a, b]);
    expect(store.items).toEqual([]);
    await store.ensureLoaded(projectId);
    expect(listModelProviders).toHaveBeenCalledTimes(2);
  });

  it('drops a stale failure', async () => {
    const first = deferred<never>();
    setClient('$modelProviderClient', { listModelProviders: vi.fn().mockReturnValueOnce(first.promise).mockResolvedValue({ modelProviders: [providerPB] }) });
    const store = useModelProvidersStore();
    const stale = store.load(projectId);
    await store.load(projectId);
    first.reject(new Error('boom'));
    await stale;
    expect(store.error).toBeNull();
    expect(store.items).toEqual([provider]);
  });

  it('creates a provider with the trimmed key and keeps no key in state', async () => {
    const createModelProvider = vi.fn().mockResolvedValue({ modelProvider: providerPB });
    setClient('$modelProviderClient', { createModelProvider });
    const store = useModelProvidersStore();
    expect(await store.add(orgId, projectId, input)).toEqual(provider);
    expect(createModelProvider.mock.calls[0]![0].apiKey).toBe('sk-or-1');
    expect(store.items).toEqual([provider]);
    expect(JSON.stringify(store.$state, (_k, v) => (typeof v === 'bigint' ? String(v) : v))).not.toContain('sk-or-1');
  });

  it('updates without a key and keeps api_key_set', async () => {
    const updateModelProvider = vi.fn().mockResolvedValue({ modelProvider: create(ModelProviderSchema, { ...providerPB, name: 'Router' }) });
    setClient('$modelProviderClient', { updateModelProvider });
    const store = useModelProvidersStore();
    store.items = [provider];
    const saved = await store.update(provider.id, { ...input, name: 'Router', apiKey: '' });
    expect(updateModelProvider.mock.calls[0]![0].apiKey).toBeUndefined();
    expect(saved.keySet).toBe(true);
    expect(store.items.map(p => p.name)).toEqual(['Router']);
  });

  it('passes on PRV006 when the URL changes without a key, keeping the old row', async () => {
    setClient('$modelProviderClient', { updateModelProvider: vi.fn().mockRejectedValue(connectError('PRV006', { reason: 'secret_required' })) });
    const store = useModelProvidersStore();
    store.items = [provider];
    const err = await store.update(provider.id, { ...input, baseUrl: 'https://other.test/v1', apiKey: '' }).catch(e => e);
    expect(errorCode(err)).toBe('PRV006');
    expect(store.items).toEqual([provider]);
  });

  it('deletes a provider, and keeps it when it is in use (PRV005)', async () => {
    const deleteModelProvider = vi.fn().mockRejectedValueOnce(connectError('PRV005', { reason: 'delete' })).mockResolvedValue({});
    setClient('$modelProviderClient', { deleteModelProvider });
    const store = useModelProvidersStore();
    store.items = [provider];
    expect(errorCode(await store.remove(provider.id).catch(e => e))).toBe('PRV005');
    expect(store.items).toEqual([provider]);
    await store.remove(provider.id);
    expect(deleteModelProvider).toHaveBeenLastCalledWith({ id: provider.id });
    expect(store.items).toEqual([]);
  });
});
