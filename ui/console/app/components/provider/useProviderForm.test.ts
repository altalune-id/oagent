import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it } from 'vitest';
import { useToast } from '#base/components/ui/toast';
import { useProvidersStore } from '~/stores/providers';
import { hasMorePrices, hasPrice, maxModels, useProviderForm } from './useProviderForm';

describe('provider helpers', () => {
  it('needs both token prices for a price', () => {
    expect(hasPrice({ id: 'a', vision: false, embedding: false, price: { input: 0, output: 1 } })).toBe(true);
    expect(hasPrice({ id: 'a', vision: false, embedding: false, price: { input: 1 } })).toBe(false);
  });

  it('opens More prices only for a row with a cache price', () => {
    const prices = { input: '1', output: '2', cacheRead: '', cacheWrite: '' };
    expect(hasMorePrices({ key: 1, id: 'a', vision: false, embedding: false, prices })).toBe(false);
    expect(hasMorePrices({ key: 1, id: 'a', vision: false, embedding: false, prices: { ...prices, cacheWrite: '0.5' } })).toBe(true);
  });
});

describe('useProviderForm', () => {
  beforeEach(async () => {
    setActivePinia(createPinia());
    await useProvidersStore().load('x6MH9WPWjktMwCBKHy4pF');
  });

  it('creates a provider with a key and priced models', async () => {
    const form = useProviderForm();
    form.show();
    expect(form.keyRequired.value).toBe(true);
    expect(form.models.value).toHaveLength(1);
    expect(await form.submit()).toBe(false);
    expect(form.errors.value).toEqual({ name: 'providers.name_required', baseUrl: 'providers.url_https', key: 'providers.key_required' });
    expect(Object.values(form.modelErrors.value)).toEqual(['providers.model_id_required']);
    form.name.value = ' Local ';
    form.baseUrl.value = 'https://llm.example.test/v1';
    form.key.value = 'sk-1';
    form.models.value[0]!.id = 'm1';
    form.models.value[0]!.prices.input = '0.4';
    form.models.value[0]!.prices.output = '1.6';
    expect(await form.submit()).toBe(true);
    const saved = useProvidersStore().items.at(-1)!;
    expect(saved).toMatchObject({ name: 'Local', keySet: true, usedBy: [], models: [{ id: 'm1', vision: false, embedding: false, price: { input: 0.4, output: 1.6 } }] });
    expect(form.open.value).toBe(false);
    expect(form.key.value).toBe('');
    expect(useToast().toasts.at(-1)?.text).toBe('providers.saved|{"name":"Local"}');
  });

  it('refuses a taken name, a duplicate model id and a negative price', async () => {
    const form = useProviderForm();
    form.show();
    form.name.value = 'openrouter';
    form.baseUrl.value = 'https://x.test';
    form.key.value = 'k';
    form.models.value[0]!.id = 'a';
    form.addModel();
    form.models.value[1]!.id = 'a';
    expect(await form.submit()).toBe(false);
    expect(form.errors.value).toEqual({ name: 'providers.name_taken' });
    expect(Object.values(form.modelErrors.value)).toEqual(['providers.model_id_taken', 'providers.model_id_taken']);
    form.removeModel(form.models.value[1]!.key);
    form.models.value[0]!.prices.cacheRead = '-1';
    await form.submit();
    expect(form.modelErrors.value).toEqual({ [form.models.value[0]!.key]: 'providers.price_invalid' });
  });

  it('edits without the key, keeps the old one, and asks for it again when the URL changes', async () => {
    const providers = useProvidersStore();
    const openrouter = providers.items[0]!;
    const form = useProviderForm();
    form.show(openrouter);
    expect(form.models.value.map(m => [m.id, m.prices.input, m.prices.cacheWrite])).toEqual([['openai/gpt-4.1-mini', '0.4', ''], ['google/gemini-2.5-flash', '0.3', ''], ['openai/text-embedding-3-small', '0.02', '']]);
    expect(form.models.value.map(m => m.embedding)).toEqual([false, false, true]);
    expect(form.keyShown.value).toBe(false);
    form.replacingKey.value = true;
    expect(form.keyShown.value).toBe(true);
    expect(form.keyRequired.value).toBe(false);
    form.baseUrl.value = 'https://other.test/v1';
    expect(form.urlChanged.value).toBe(true);
    expect(await form.submit()).toBe(false);
    expect(form.errors.value).toEqual({ key: 'providers.key_required' });
    form.key.value = 'new';
    expect(await form.submit()).toBe(true);
    expect(providers.items[0]).toMatchObject({ id: openrouter.id, baseUrl: 'https://other.test/v1', usedBy: openrouter.usedBy });
  });

  it('caps the model rows and closes on hide', () => {
    const form = useProviderForm();
    form.show();
    for (let i = 0; i < maxModels + 5; i++) {
      form.addModel();
    }
    expect(form.models.value).toHaveLength(maxModels);
    form.key.value = 'typed';
    form.hide();
    expect(form.open.value).toBe(false);
    expect(form.key.value).toBe('');
  });
});
