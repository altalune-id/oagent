import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it } from 'vitest';
import { nextTick } from 'vue';
import { useWebSearchStore } from '~/stores/webSearch';
import { searchProviders, useWebSearchForm } from './useWebSearchForm';

describe('useWebSearchForm', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('offers three providers', () => {
    expect(searchProviders.map(p => p.id)).toEqual(['brave', 'exa', 'tavily']);
  });

  it('needs a key to save, then hides the key until the provider changes or the key is replaced', async () => {
    const store = useWebSearchStore();
    const form = useWebSearchForm();
    expect(form.choice.value).toBe('brave');
    expect(form.keyShown.value).toBe(true);
    expect(form.save()).toBe(false);
    expect(form.error.value).toBe('websearch.key_required');
    form.choice.value = 'tavily';
    form.key.value = 'tvly-1';
    expect(form.save()).toBe(true);
    expect(store.setting.provider).toBe('tavily');
    expect(form.keyShown.value).toBe(false);
    form.choice.value = 'exa';
    expect(form.keyShown.value).toBe(true);
    form.cancel();
    expect(form.choice.value).toBe('tavily');
    form.replacing.value = true;
    expect(form.keyShown.value).toBe(true);
    store.remove();
    await nextTick();
    expect(form.choice.value).toBe('tavily');
    store.save('exa');
    await nextTick();
    expect(form.choice.value).toBe('exa');
  });

  it('starts from the saved provider', () => {
    useWebSearchStore().save('exa');
    expect(useWebSearchForm().choice.value).toBe('exa');
  });
});
