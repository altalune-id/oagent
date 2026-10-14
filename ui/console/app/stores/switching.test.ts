import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { connectError, deferred, setClient, setKnownKeys } from '../../test/nuxt-globals';
import { useAgentsStore } from './agents';
import { useAPIKeysStore } from './apiKeys';
import { useProjectsStore } from './projects';
import { useWebhooksStore } from './webhooks';

interface Case {
  name: string;
  client: '$projectClient' | '$agentClient' | '$apiKeyClient' | '$webhookClient';
  method: string;
  field: string;
  item: (id: string) => Record<string, unknown>;
  key: 'id' | 'secretHint';
  store: () => { items: Partial<Record<'id' | 'secretHint', string>>[], loading: boolean, error: unknown, ensureLoaded: (...ids: string[]) => Promise<void> };
  ids: [string[], string[]];
}

const cases: Case[] = [
  {
    name: 'projects',
    client: '$projectClient',
    method: 'listProjects',
    field: 'projects',
    key: 'id',
    item: id => ({ id, orgId: 'o', slug: id, name: id }),
    store: () => useProjectsStore(),
    ids: [['V3StGXR8Z5jdHj6BmyT2a'], ['YeDxg2kMYznPJJG3wVmjj']],
  },
  {
    name: 'agents',
    client: '$agentClient',
    method: 'listAgents',
    field: 'agents',
    key: 'id',
    item: id => ({ id, projectId: 'p', slug: id, name: id, description: '' }),
    store: () => useAgentsStore(),
    ids: [['x6MH9WPWjktMwCBKHy4pF'], ['KdrF3t3FHtppBPmMgj4sy']],
  },
  {
    name: 'apiKeys',
    client: '$apiKeyClient',
    method: 'listAPIKeys',
    field: 'keys',
    key: 'secretHint',
    item: id => ({ secretHint: id, name: id, kind: 1, scopes: [], projectIds: [] }),
    store: () => useAPIKeysStore(),
    ids: [['V3StGXR8Z5jdHj6BmyT2a'], ['YeDxg2kMYznPJJG3wVmjj']],
  },
  {
    name: 'webhooks',
    client: '$webhookClient',
    method: 'listWebhooks',
    field: 'webhooks',
    key: 'id',
    item: id => ({ id, projectId: 'p', url: 'https://x', eventTypes: [], active: true }),
    store: () => useWebhooksStore(),
    ids: [['V3StGXR8Z5jdHj6BmyT2a', 'x6MH9WPWjktMwCBKHy4pF'], ['V3StGXR8Z5jdHj6BmyT2a', 'KdrF3t3FHtppBPmMgj4sy']],
  },
];

describe.each(cases)('$name store when the selection changes mid-load', (c) => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('loads the new selection instead of sharing the old request', async () => {
    const first = deferred<Record<string, unknown>>();
    const second = deferred<Record<string, unknown>>();
    const list = vi.fn().mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
    setClient(c.client, { [c.method]: list });
    const store = c.store();

    const a = store.ensureLoaded(...c.ids[0]);
    const b = store.ensureLoaded(...c.ids[1]);
    expect(list).toHaveBeenCalledTimes(2);

    second.resolve({ [c.field]: [c.item('new')] });
    await b;
    first.resolve({ [c.field]: [c.item('old')] });
    await a;

    expect(store.items.map(i => i[c.key])).toEqual(['new']);
    expect(store.loading).toBe(false);
  });

  it('ignores a late failure of the old request', async () => {
    setKnownKeys('apperror.GEN900');
    const first = deferred<Record<string, unknown>>();
    const list = vi.fn().mockReturnValueOnce(first.promise).mockResolvedValueOnce({ [c.field]: [c.item('new')] });
    setClient(c.client, { [c.method]: list });
    const store = c.store();

    const a = store.ensureLoaded(...c.ids[0]);
    await store.ensureLoaded(...c.ids[1]);
    first.reject(connectError('GEN900'));
    await a;

    expect(store.error).toBeNull();
    expect(store.items.map(i => i[c.key])).toEqual(['new']);
  });
});
