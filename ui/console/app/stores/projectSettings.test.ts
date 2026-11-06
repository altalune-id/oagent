import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it } from 'vitest';
import { useWebSearchStore } from './webSearch';

const projectId = 'x6MH9WPWjktMwCBKHy4pF';

describe('web search store', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('starts with no search provider, saves one with its date and removes it', async () => {
    const search = useWebSearchStore();
    await search.ensureLoaded(projectId);
    await search.ensureLoaded(projectId);
    expect(search.setting.provider).toBe('');
    const at = new Date('2026-11-04T10:00:00Z');
    search.save('exa', at);
    expect(search.setting).toMatchObject({ provider: 'exa', keyAddedAt: at, usedBy: ['Tagihan v2'] });
    search.remove();
    expect(search.setting).toEqual({ provider: '', usedBy: ['Tagihan v2'] });
  });
});
