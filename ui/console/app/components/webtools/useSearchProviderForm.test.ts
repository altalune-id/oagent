import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { nextTick } from 'vue';
import { useToast } from '#base/components/ui/toast';
import { WebSearchProviderKind } from '#gen/webtool/v1/webtool_pb';
import { connectError, setClient, setKnownKeys } from '#test/nuxt-globals';
import { useWebToolsStore } from '~/stores/webTools';
import { searchProviders, useSearchProviderForm } from './useSearchProviderForm';

const projectId = 'x6MH9WPWjktMwCBKHy4pF';
const { BRAVE, EXA, TAVILY } = WebSearchProviderKind;

describe('useSearchProviderForm', () => {
  let setWebSearchProvider: ReturnType<typeof vi.fn>;
  let deleteWebSearchProvider: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    setActivePinia(createPinia());
    setWebSearchProvider = vi.fn(async (req: { kind: WebSearchProviderKind, pricePerCallMicros: bigint }) => ({ provider: { kind: req.kind, keySet: true, pricePerCallMicros: req.pricePerCallMicros } }));
    deleteWebSearchProvider = vi.fn().mockResolvedValue({});
    setClient('$webToolClient', { setWebSearchProvider, deleteWebSearchProvider });
    useWebToolsStore().availableKinds = [BRAVE, EXA];
  });

  function stored(kind: WebSearchProviderKind, pricePerCallMicros = 8125n) {
    useWebToolsStore().provider = { kind, keySet: true, pricePerCallMicros };
  }

  it('lists every kind and selects only the available ones', () => {
    const form = useSearchProviderForm(projectId);
    expect(searchProviders.map(p => p.kind)).toEqual([BRAVE, EXA, TAVILY]);
    expect([BRAVE, EXA, TAVILY].map(form.selectable)).toEqual([true, true, false]);
  });

  it('needs a key and a price for a new provider, then saves it', async () => {
    const form = useSearchProviderForm(projectId);
    expect(form.kind.value).toBe(BRAVE);
    expect(form.keyShown.value).toBe(true);
    expect(await form.submit()).toBe(false);
    expect(form.errors.value).toEqual({ api_key: 'webtools.key_required', price_per_call_micros: 'webtools.price_invalid' });
    expect(setWebSearchProvider).not.toHaveBeenCalled();
    form.apiKey.value = ' bsa-1 ';
    form.price.value = '5';
    expect(await form.submit()).toBe(true);
    expect(setWebSearchProvider).toHaveBeenCalledWith({ projectId, kind: BRAVE, apiKey: 'bsa-1', pricePerCallMicros: 5000n });
    expect(useToast().toasts.at(-1)?.text).toBe('webtools.saved');
    expect(form.apiKey.value).toBe('');
    expect(form.keyShown.value).toBe(false);
  });

  it('reads an id-ID price with a decimal comma', async () => {
    useI18n().locale.value = 'id-ID';
    const form = useSearchProviderForm(projectId);
    form.apiKey.value = 'k';
    form.price.value = '5,00';
    expect(await form.submit()).toBe(true);
    expect(setWebSearchProvider).toHaveBeenCalledWith({ projectId, kind: BRAVE, apiKey: 'k', pricePerCallMicros: 5000n });
    useI18n().locale.value = 'en-US';
  });

  it('refuses a price with more than three decimals', async () => {
    const form = useSearchProviderForm(projectId);
    form.apiKey.value = 'k';
    form.price.value = '8.1255';
    expect(await form.submit()).toBe(false);
    expect(form.errors.value).toEqual({ price_per_call_micros: 'webtools.price_invalid' });
  });

  it('prefills the saved provider and keeps its key when only the price changes', async () => {
    stored(EXA);
    const form = useSearchProviderForm(projectId);
    expect(form.kind.value).toBe(EXA);
    expect(form.price.value).toBe('8.125');
    expect(form.keyShown.value).toBe(false);
    form.price.value = '0';
    expect(await form.submit()).toBe(true);
    expect(setWebSearchProvider).toHaveBeenCalledWith({ projectId, kind: EXA, pricePerCallMicros: 0n });
  });

  it('needs a key to switch kinds or to replace the key', async () => {
    stored(EXA);
    const form = useSearchProviderForm(projectId);
    form.kind.value = BRAVE;
    expect(form.keyShown.value).toBe(true);
    expect(await form.submit()).toBe(false);
    expect(form.errors.value).toEqual({ api_key: 'webtools.key_required' });
    form.cancel();
    expect([form.kind.value, form.keyShown.value]).toEqual([EXA, false]);
    form.replacing.value = true;
    expect(await form.submit()).toBe(false);
    form.apiKey.value = 'new';
    expect(await form.submit()).toBe(true);
    expect(setWebSearchProvider).toHaveBeenCalledWith({ projectId, kind: EXA, apiKey: 'new', pricePerCallMicros: 8125n });
    expect(form.replacing.value).toBe(false);
  });

  it('starts a stored unavailable kind on the first selectable kind and needs a key', async () => {
    stored(TAVILY);
    const form = useSearchProviderForm(projectId);
    expect(form.kind.value).toBe(BRAVE);
    expect(form.price.value).toBe('');
    expect(form.keyShown.value).toBe(true);
  });

  it('shows the form only without a usable provider, or while changing or replacing it', async () => {
    const form = useSearchProviderForm(projectId);
    expect(form.formShown.value).toBe(true);
    stored(EXA);
    await nextTick();
    expect(form.formShown.value).toBe(false);
    form.editing.value = true;
    expect(form.formShown.value).toBe(true);
    form.cancel();
    expect(form.formShown.value).toBe(false);
    form.replacing.value = true;
    expect(form.formShown.value).toBe(true);
    form.apiKey.value = 'k';
    expect(await form.submit()).toBe(true);
    expect(form.formShown.value).toBe(false);
  });

  it('starts on the first available kind', () => {
    useWebToolsStore().availableKinds = [EXA];
    expect(useSearchProviderForm(projectId).kind.value).toBe(EXA);
  });

  it('never sends a kind that is not selectable', async () => {
    const form = useSearchProviderForm(projectId);
    form.kind.value = TAVILY;
    form.apiKey.value = 'k';
    form.price.value = '1';
    expect(await form.submit()).toBe(false);
    expect(setWebSearchProvider).not.toHaveBeenCalled();
  });

  it('places WEB001 under the kind and WEB002 under the key, and shows WEB003 on the form', async () => {
    setKnownKeys('apperror.WEB001', 'apperror.WEB002', 'apperror.WEB003');
    setWebSearchProvider
      .mockRejectedValueOnce(connectError('WEB001', { kind: 'tavily' }))
      .mockRejectedValueOnce(connectError('WEB002'))
      .mockRejectedValueOnce(connectError('WEB003'));
    const form = useSearchProviderForm(projectId);
    form.apiKey.value = 'k';
    form.price.value = '1';
    expect(await form.submit()).toBe(false);
    expect(form.errors.value).toEqual({ kind: 'apperror.WEB001|{"kind":"tavily"}' });
    expect(await form.submit()).toBe(false);
    expect(form.errors.value).toEqual({ api_key: 'apperror.WEB002' });
    expect(await form.submit()).toBe(false);
    expect(form.errors.value).toEqual({});
    expect(form.error.value?.message).toBe('apperror.WEB003');
  });

  it('follows the store when the saved provider changes', async () => {
    const form = useSearchProviderForm(projectId);
    form.apiKey.value = 'typed';
    stored(EXA, 1000n);
    await nextTick();
    expect([form.kind.value, form.price.value, form.apiKey.value]).toEqual([EXA, '1.00', '']);
  });

  it('removes the provider through a confirm', async () => {
    stored(EXA);
    const form = useSearchProviderForm(projectId);
    form.removing.ask(true);
    expect(await form.removing.confirm()).toBe(true);
    expect(deleteWebSearchProvider).toHaveBeenCalledWith({ projectId });
    expect(useWebToolsStore().provider).toBeNull();
    expect(useToast().toasts.at(-1)?.text).toBe('webtools.removed');
  });
});
