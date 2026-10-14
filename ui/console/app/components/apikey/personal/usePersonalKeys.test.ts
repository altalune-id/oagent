import { create } from '@bufbuild/protobuf';
import { timestampDate } from '@bufbuild/protobuf/wkt';
import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { useAPIKeysStore } from '~/stores/apiKeys';
import { useAuthStore } from '~/stores/auth';
import { APIKeySchema } from '../../../../gen/apikey/v1/apikey_pb';
import { connectError, runtimeConfig, setClient, setKnownKeys } from '../../../../test/nuxt-globals';
import { mcpSnippet } from './mcpSnippet';
import { usePersonalKeys } from './usePersonalKeys';

const now = new Date('2026-10-01T00:00:00Z');
const personalPB = create(APIKeySchema, { secretHint: 'aaaa', kind: 'personal', name: 'Mine', scopes: ['agent:read', 'mystery:do'] });
const orgPB = create(APIKeySchema, { secretHint: 'bbbb', kind: 'org', name: 'CI', scopes: ['agent:read'] });
const form = { name: 'Laptop', scopes: ['agent:read'], expiresInHours: 24, allProjects: true, projectIds: ['x6MH9WPWjktMwCBKHy4pF'] };

describe('usePersonalKeys', () => {
  let client: Record<string, ReturnType<typeof vi.fn>>;
  let whoami: ReturnType<typeof vi.fn>;
  let listProjects: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    setActivePinia(createPinia());
    vi.useFakeTimers();
    vi.setSystemTime(now);
    vi.stubGlobal('window', { location: { origin: 'https://console.test' } });
    client = {
      listAPIKeys: vi.fn().mockResolvedValue({ keys: [personalPB, orgPB] }),
      createAPIKey: vi.fn().mockResolvedValue({ key: create(APIKeySchema, { secretHint: 'cccc', kind: 'personal', name: 'Laptop' }), plaintext: 'oak_new' }),
      revokeAPIKey: vi.fn().mockResolvedValue({}),
    };
    whoami = vi.fn().mockResolvedValue({ source: 'local', permissions: ['agent:*', 'member:write'] });
    listProjects = vi.fn().mockResolvedValue({ projects: [{ id: 'x6MH9WPWjktMwCBKHy4pF', orgId: 'V3StGXR8Z5jdHj6BmyT2a', slug: 'web', name: 'Web' }, { id: 'KdrF3t3FHtppBPmMgj4sy', orgId: 'YeDxg2kMYznPJJG3wVmjj', slug: 'x', name: 'X' }] });
    setClient('$apiKeyClient', client);
    setClient('$authClient', { whoami });
    setClient('$projectClient', { listProjects });
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('loads the viewer, the keys and the projects', async () => {
    const keys = usePersonalKeys('V3StGXR8Z5jdHj6BmyT2a');
    expect(keys.signedIn.value).toBe(false);
    expect(keys.mintable.value).toEqual([]);
    await keys.load();
    expect(client.listAPIKeys).toHaveBeenCalledWith({ orgId: 'V3StGXR8Z5jdHj6BmyT2a' });
    expect(listProjects).toHaveBeenCalledWith({ orgId: 'V3StGXR8Z5jdHj6BmyT2a' });
    expect(keys.signedIn.value).toBe(true);
    expect(keys.mine.value.map(k => [k.secretHint, k.scopes])).toEqual([['aaaa', ['scope.agent_read', 'mystery:do']]]);
    expect(keys.mintable.value).toEqual([
      { name: 'agent:read', label: 'scope.agent_read' },
      { name: 'agent:write', label: 'scope.agent_write' },
    ]);
    expect(keys.orgProjects.value).toEqual([{ id: 'x6MH9WPWjktMwCBKHy4pF', name: 'Web' }]);
  });

  it('is not signed in for a non-local source', async () => {
    whoami.mockResolvedValue({ source: 'apikey', permissions: [] });
    const keys = usePersonalKeys('V3StGXR8Z5jdHj6BmyT2a');
    await keys.load();
    expect(keys.signedIn.value).toBe(false);
  });

  it('creates a key for all projects and builds the MCP snippet', async () => {
    runtimeConfig.public.apiUrl = '/base/api';
    const keys = usePersonalKeys('V3StGXR8Z5jdHj6BmyT2a');
    await keys.create(form);
    const req = client.createAPIKey!.mock.calls[0]![0];
    expect(req).toMatchObject({ orgId: 'V3StGXR8Z5jdHj6BmyT2a', kind: 'personal', name: 'Laptop', scopes: ['agent:read'], allProjects: true, projectIds: [] });
    expect(timestampDate(req.expiresAt)).toEqual(new Date(now.getTime() + 24 * 3600_000));
    expect(useAPIKeysStore().items.map(k => k.secretHint)).toEqual(['cccc']);
    expect(keys.created.value).toEqual({ plaintext: 'oak_new', snippet: mcpSnippet('oagent', 'https://console.test/base/mcp', 'oak_new') });
    expect(keys.busy.value).toBe(false);
    keys.dismiss();
    expect(keys.created.value).toBeNull();
  });

  it('sends chosen projects and skips the store when no key comes back', async () => {
    client.createAPIKey!.mockResolvedValue({ plaintext: 'oak_x' });
    const keys = usePersonalKeys('V3StGXR8Z5jdHj6BmyT2a');
    await keys.create({ ...form, allProjects: false, projectIds: ['x6MH9WPWjktMwCBKHy4pF', 'KdrF3t3FHtppBPmMgj4sy'] });
    expect(client.createAPIKey!.mock.calls[0]![0]).toMatchObject({ allProjects: false, projectIds: ['x6MH9WPWjktMwCBKHy4pF', 'KdrF3t3FHtppBPmMgj4sy'] });
    expect(useAPIKeysStore().items).toEqual([]);
    expect(keys.created.value?.plaintext).toBe('oak_x');
  });

  it('shows create errors', async () => {
    setKnownKeys('apperror.GEN007');
    client.createAPIKey!.mockRejectedValueOnce(connectError('GEN007', { permission: 'agent:write' })).mockRejectedValueOnce(new Error('boom'));
    const keys = usePersonalKeys('V3StGXR8Z5jdHj6BmyT2a');
    await keys.create(form);
    expect(keys.error.value).toEqual({ message: 'apperror.GEN007|{"permission":"scope.agent_write"}', reference: 'error.ref|{"id":"req-1"}' });
    expect(keys.created.value).toBeNull();
    await keys.create(form);
    expect(keys.error.value).toEqual({ message: 'error.create_apikey' });
    expect(keys.busy.value).toBe(false);
  });

  it('revokes a key by its hint', async () => {
    const keys = usePersonalKeys('V3StGXR8Z5jdHj6BmyT2a');
    await keys.load();
    await keys.revoke('aaaa');
    expect(client.revokeAPIKey).toHaveBeenCalledWith({ orgId: 'V3StGXR8Z5jdHj6BmyT2a', hint: 'aaaa' });
    expect(useAPIKeysStore().items.find(k => k.secretHint === 'aaaa')?.revokedAt).toEqual(now);
    expect(keys.error.value).toBeNull();
  });

  it('shows revoke errors and keeps the key active', async () => {
    setKnownKeys('apperror.GEN900');
    client.revokeAPIKey!.mockRejectedValueOnce(connectError('GEN900')).mockRejectedValueOnce(new Error('boom'));
    const keys = usePersonalKeys('V3StGXR8Z5jdHj6BmyT2a');
    await keys.load();
    await keys.revoke('aaaa');
    expect(keys.error.value).toEqual({ message: 'apperror.GEN900', reference: 'error.ref|{"id":"req-1"}' });
    await keys.revoke('aaaa');
    expect(keys.error.value).toEqual({ message: 'error.revoke_apikey' });
    expect(useAPIKeysStore().items.find(k => k.secretHint === 'aaaa')?.revokedAt).toBeUndefined();
    expect(useAuthStore().me).not.toBeNull();
  });
});
