import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it } from 'vitest';
import { useMcpServersStore } from './mcpServers';
import { useProvidersStore } from './providers';
import { useWebSearchStore } from './webSearch';

const projectId = 'x6MH9WPWjktMwCBKHy4pF';

describe('project settings stores', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('loads the sample providers once per project, then adds, replaces and removes', async () => {
    const providers = useProvidersStore();
    await providers.ensureLoaded(projectId);
    expect(providers.items.map(p => p.name)).toEqual(['OpenRouter', 'Groq']);
    providers.items[0]!.name = 'Changed';
    await providers.ensureLoaded(projectId);
    expect(providers.items[0]!.name).toBe('Changed');
    const groq = providers.items[1]!;
    providers.save({ ...groq, name: 'Groq Cloud' });
    providers.save({ ...groq, id: 'prv_new', name: 'Local' });
    expect(providers.items.map(p => p.name)).toEqual(['Changed', 'Groq Cloud', 'Local']);
    providers.remove('prv_new');
    expect(providers.items).toHaveLength(2);
    expect(providers.loading).toBe(false);
  });

  it('loads the sample MCP servers, then adds, replaces and removes', async () => {
    const servers = useMcpServersStore();
    await servers.ensureLoaded(projectId);
    await servers.ensureLoaded(projectId);
    const yasaku = servers.items[0]!;
    expect(yasaku.name).toBe('yasaku');
    servers.save({ ...yasaku, timeoutSeconds: 30 });
    servers.save({ ...yasaku, id: 'mcs_new', name: 'other' });
    expect(servers.items.map(s => [s.name, s.timeoutSeconds])).toEqual([['yasaku', 30], ['tagihan', 10], ['other', 20]]);
    servers.remove('mcs_new');
    expect(servers.items).toHaveLength(2);
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
