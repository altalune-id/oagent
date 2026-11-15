import { create } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { HTTPSecretSchema, WebSearchProviderKind, WebSearchProviderSchema } from '#gen/webtool/v1/webtool_pb';
import { connectError, deferred, setClient, setKnownKeys } from '#test/nuxt-globals';
import { useWebToolsStore } from './webTools';

const projectId = 'x6MH9WPWjktMwCBKHy4pF';
const at = new Date('2026-11-01T10:00:00Z');
const providerPB = create(WebSearchProviderSchema, { kind: WebSearchProviderKind.BRAVE, keySet: true, pricePerCallMicros: 5000n, updatedAt: timestampFromDate(at) });
const secretPB = create(HTTPSecretSchema, { id: 'UxVy4UK3vmrHzPZ273hCA', host: 'api.example.com', header: 'x-api-key', updatedAt: timestampFromDate(at) });
const available = [WebSearchProviderKind.BRAVE, WebSearchProviderKind.EXA];

function fakeClient(over: Record<string, unknown> = {}) {
  const client = {
    getWebSearchProvider: vi.fn().mockResolvedValue({ provider: providerPB, availableKinds: available, usedBy: [{ id: 'a6SkMeA9Zj2rjRmCXx2dM', name: 'Support' }] }),
    listHTTPSecrets: vi.fn().mockResolvedValue({ secrets: [secretPB] }),
    ...over,
  };
  setClient('$webToolClient', client);
  return client;
}

describe('useWebToolsStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('loads the provider and the secrets of a project', async () => {
    const client = fakeClient();
    const store = useWebToolsStore();
    expect(store.searchReady).toBeUndefined();
    await store.load(projectId);
    expect(client.getWebSearchProvider).toHaveBeenCalledWith({ projectId });
    expect(client.listHTTPSecrets).toHaveBeenCalledWith({ projectId });
    expect(store.provider).toEqual({ kind: WebSearchProviderKind.BRAVE, keySet: true, pricePerCallMicros: 5000n, updatedAt: at });
    expect(store.availableKinds).toEqual(available);
    expect(store.usedBy).toEqual([{ id: 'a6SkMeA9Zj2rjRmCXx2dM', name: 'Support' }]);
    expect(store.secrets).toEqual([{ id: 'UxVy4UK3vmrHzPZ273hCA', host: 'api.example.com', header: 'x-api-key', updatedAt: at }]);
    expect(store.usableProvider).toEqual(store.provider);
    expect(store.searchReady).toBe(true);
    expect(store.loading).toBe(false);
  });

  it('reads no provider and missing lists as empty', async () => {
    fakeClient({ getWebSearchProvider: vi.fn().mockResolvedValue({}), listHTTPSecrets: vi.fn().mockResolvedValue({}) });
    const store = useWebToolsStore();
    await store.load(projectId);
    expect(store.provider).toBeNull();
    expect(store.availableKinds).toEqual([]);
    expect(store.usedBy).toEqual([]);
    expect(store.secrets).toEqual([]);
    expect(store.searchReady).toBe(false);
  });

  it('treats a stored kind that is no longer available as no provider', async () => {
    fakeClient({ getWebSearchProvider: vi.fn().mockResolvedValue({ provider: { ...providerPB, kind: WebSearchProviderKind.TAVILY }, availableKinds: available, usedBy: [] }) });
    const store = useWebToolsStore();
    await store.load(projectId);
    expect(store.provider?.kind).toBe(WebSearchProviderKind.TAVILY);
    expect(store.usableProvider).toBeNull();
    expect(store.searchReady).toBe(false);
  });

  it('shows a load error and leaves readiness unknown', async () => {
    setKnownKeys('apperror.GEN900');
    fakeClient({ getWebSearchProvider: vi.fn().mockRejectedValue(connectError('GEN900')) });
    const store = useWebToolsStore();
    await store.load(projectId);
    expect(store.error).toEqual({ message: 'apperror.GEN900', reference: 'error.ref|{"id":"req-1"}' });
    expect(store.searchReady).toBeUndefined();
    expect(store.loading).toBe(false);
  });

  it('drops a stale answer', async () => {
    const first = deferred<unknown>();
    const getWebSearchProvider = vi.fn()
      .mockImplementationOnce(() => first.promise)
      .mockResolvedValueOnce({ availableKinds: available, usedBy: [] });
    fakeClient({ getWebSearchProvider });
    const store = useWebToolsStore();
    const stale = store.load(projectId);
    await store.load('KdrF3t3FHtppBPmMgj4sy');
    first.resolve({ provider: providerPB, availableKinds: available, usedBy: [] });
    await stale;
    expect(store.provider).toBeNull();
    expect(store.loading).toBe(false);
  });

  it('drops a stale error', async () => {
    const first = deferred<unknown>();
    const getWebSearchProvider = vi.fn()
      .mockImplementationOnce(() => first.promise)
      .mockResolvedValueOnce({ availableKinds: available, usedBy: [] });
    fakeClient({ getWebSearchProvider });
    const store = useWebToolsStore();
    const stale = store.load(projectId);
    await store.load('KdrF3t3FHtppBPmMgj4sy');
    first.reject(new Error('boom'));
    await stale;
    expect(store.error).toBeNull();
  });

  it('loads a project again after another project failed to load', async () => {
    const getWebSearchProvider = vi.fn()
      .mockResolvedValueOnce({ availableKinds: available, usedBy: [] })
      .mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValueOnce({ provider: providerPB, availableKinds: available, usedBy: [] });
    fakeClient({ getWebSearchProvider });
    const store = useWebToolsStore();
    await store.ensureLoaded(projectId);
    await store.ensureLoaded('KdrF3t3FHtppBPmMgj4sy');
    expect(store.error).not.toBeNull();
    await store.ensureLoaded(projectId);
    expect(getWebSearchProvider).toHaveBeenCalledTimes(3);
    expect(store.error).toBeNull();
    expect(store.searchReady).toBe(true);
  });

  it('shares an in-flight load and skips a project already loaded', async () => {
    const pending = deferred<unknown>();
    const client = fakeClient({ getWebSearchProvider: vi.fn(() => pending.promise) });
    const store = useWebToolsStore();
    await store.ensureLoaded('');
    const a = store.ensureLoaded(projectId);
    const b = store.ensureLoaded(projectId);
    expect(client.getWebSearchProvider).toHaveBeenCalledTimes(1);
    pending.resolve({ availableKinds: available, usedBy: [] });
    await Promise.all([a, b]);
    await store.ensureLoaded(projectId);
    expect(client.getWebSearchProvider).toHaveBeenCalledTimes(1);
  });

  it('keeps the saved provider from the answer and clears it on delete', async () => {
    const setWebSearchProvider = vi.fn().mockResolvedValue({ provider: { ...providerPB, kind: WebSearchProviderKind.EXA } });
    const deleteWebSearchProvider = vi.fn().mockResolvedValue({});
    fakeClient({ setWebSearchProvider, deleteWebSearchProvider });
    const store = useWebToolsStore();
    await store.load(projectId);
    await store.setProvider({ projectId, kind: WebSearchProviderKind.EXA, apiKey: 'k', pricePerCallMicros: 5000n });
    expect(setWebSearchProvider).toHaveBeenCalledWith({ projectId, kind: WebSearchProviderKind.EXA, apiKey: 'k', pricePerCallMicros: 5000n });
    expect(store.provider?.kind).toBe(WebSearchProviderKind.EXA);
    await store.deleteProvider(projectId);
    expect(deleteWebSearchProvider).toHaveBeenCalledWith({ projectId });
    expect(store.provider).toBeNull();
    expect(store.usedBy).toEqual([{ id: 'a6SkMeA9Zj2rjRmCXx2dM', name: 'Support' }]);
  });

  it('upserts a saved secret and removes a deleted one', async () => {
    const other = create(HTTPSecretSchema, { id: 'a6SkMeA9Zj2rjRmCXx2dM', host: 'b.example.com', header: 'authorization' });
    const setHTTPSecret = vi.fn()
      .mockResolvedValueOnce({ secret: { ...secretPB, updatedAt: undefined } })
      .mockResolvedValueOnce({ secret: other });
    const deleteHTTPSecret = vi.fn().mockResolvedValue({});
    fakeClient({ setHTTPSecret, deleteHTTPSecret });
    const store = useWebToolsStore();
    await store.load(projectId);
    await store.setSecret({ projectId, host: 'api.example.com', header: 'x-api-key', value: 'v' });
    expect(store.secrets).toEqual([{ id: 'UxVy4UK3vmrHzPZ273hCA', host: 'api.example.com', header: 'x-api-key', updatedAt: undefined }]);
    await store.setSecret({ projectId, host: 'b.example.com', header: 'authorization', value: 'v' });
    expect(store.secrets.map(s => s.id)).toEqual(['UxVy4UK3vmrHzPZ273hCA', 'a6SkMeA9Zj2rjRmCXx2dM']);
    await store.deleteSecret('UxVy4UK3vmrHzPZ273hCA');
    expect(deleteHTTPSecret).toHaveBeenCalledWith({ id: 'UxVy4UK3vmrHzPZ273hCA' });
    expect(store.secrets.map(s => s.id)).toEqual(['a6SkMeA9Zj2rjRmCXx2dM']);
  });

  it('drops a secret the server no longer has and rethrows other delete errors', async () => {
    const deleteHTTPSecret = vi.fn()
      .mockRejectedValueOnce(connectError('WEB004'))
      .mockRejectedValueOnce(connectError('GEN900'));
    fakeClient({ deleteHTTPSecret });
    const store = useWebToolsStore();
    await store.load(projectId);
    await store.deleteSecret('UxVy4UK3vmrHzPZ273hCA');
    expect(store.secrets).toEqual([]);
    await store.load(projectId);
    await expect(store.deleteSecret('UxVy4UK3vmrHzPZ273hCA')).rejects.toThrow();
    expect(store.secrets).toHaveLength(1);
  });
});
