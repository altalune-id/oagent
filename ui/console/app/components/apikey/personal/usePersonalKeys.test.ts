import { create } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { APIKeySchema } from '#gen/apikey/v1/apikey_pb';
import { connectError, setClient, setKnownKeys } from '#test/nuxt-globals';
import { useAPIKeysStore } from '~/stores/apiKeys';
import { useAuthStore } from '~/stores/auth';
import { usePersonalKeys } from './usePersonalKeys';

const orgId = 'V3StGXR8Z5jdHj6BmyT2a';
const now = new Date('2026-10-01T00:00:00Z');
const day = 86_400_000;
const personalPB = create(APIKeySchema, {
  secretHint: 'aaaa',
  kind: 'personal',
  name: 'Mine',
  scopes: ['agent:read', 'mystery:do'],
  expiresAt: timestampFromDate(new Date(now.getTime() + 3 * day)),
  lastUsedAt: timestampFromDate(new Date(now.getTime() - 2 * day)),
});
const laterPB = create(APIKeySchema, { secretHint: 'dddd', kind: 'personal', name: 'Server', expiresAt: timestampFromDate(new Date('2027-03-01T00:00:00Z')) });
const orgPB = create(APIKeySchema, { secretHint: 'bbbb', kind: 'org', name: 'CI', scopes: ['agent:read'] });

describe('usePersonalKeys', () => {
  let client: Record<string, ReturnType<typeof vi.fn>>;
  let whoami: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    setActivePinia(createPinia());
    vi.useFakeTimers();
    vi.setSystemTime(now);
    client = {
      listAPIKeys: vi.fn().mockResolvedValue({ keys: [personalPB, laterPB, orgPB] }),
      revokeAPIKey: vi.fn().mockResolvedValue({}),
    };
    whoami = vi.fn().mockResolvedValue({ source: 'local', userId: 'F5pLm2XwQ8kRt3VnZ7bYc', permissions: ['agent:*'] });
    setClient('$apiKeyClient', client);
    setClient('$authClient', { whoami });
    setClient('$projectClient', { listProjects: vi.fn().mockResolvedValue({ projects: [] }) });
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('lists only the viewer\'s personal keys, with last use and expiry', async () => {
    const keys = usePersonalKeys(orgId);
    expect(keys.signedIn.value).toBe(false);
    await keys.load();
    expect(client.listAPIKeys).toHaveBeenCalledWith({ orgId });
    expect(keys.signedIn.value).toBe(true);
    expect(keys.rows.value.map(({ rowKey: _, ...k }) => k)).toEqual([
      { name: 'Mine', secretHint: 'aaaa', scopes: ['scope.agent_read', 'mystery:do'], lastUsed: '2 days ago', expires: 'Oct 4, 2026', expiry: { kind: 'soon', days: 3 }, revoked: false },
      { name: 'Server', secretHint: 'dddd', scopes: [], lastUsed: '', expires: 'Mar 1, 2027', expiry: { kind: 'ok' }, revoked: false },
    ]);
  });

  it('is signed in through the OIDC provider too, and not for a non-person source', async () => {
    whoami.mockResolvedValueOnce({ source: 'oidc', userId: 'F5pLm2XwQ8kRt3VnZ7bYc', permissions: [] });
    const keys = usePersonalKeys(orgId);
    await keys.load();
    expect(keys.signedIn.value).toBe(true);
    setActivePinia(createPinia());
    whoami.mockResolvedValueOnce({ source: 'apikey', permissions: [] });
    const other = usePersonalKeys(orgId);
    await other.load();
    expect(other.signedIn.value).toBe(false);
  });

  it('holds a new key\'s secret until it is dismissed', () => {
    const keys = usePersonalKeys(orgId);
    keys.created.value = { plaintext: 'oak_new', snippet: '{}' };
    keys.dismiss();
    expect(keys.created.value).toBeNull();
  });

  it('revokes a key by its hint', async () => {
    const keys = usePersonalKeys(orgId);
    await keys.load();
    await keys.revoke('aaaa');
    expect(client.revokeAPIKey).toHaveBeenCalledWith({ orgId, hint: 'aaaa' });
    expect(useAPIKeysStore().items.find(k => k.secretHint === 'aaaa')?.revokedAt).toEqual(now);
    expect(keys.rows.value.find(k => k.secretHint === 'aaaa')?.revoked).toBe(true);
    expect(keys.error.value).toBeNull();
  });

  it('shows revoke errors and keeps the key active', async () => {
    setKnownKeys('apperror.GEN900');
    client.revokeAPIKey!.mockRejectedValueOnce(connectError('GEN900')).mockRejectedValueOnce(new Error('boom'));
    const keys = usePersonalKeys(orgId);
    await keys.load();
    await keys.revoke('aaaa');
    expect(keys.error.value).toEqual({ message: 'apperror.GEN900', reference: 'error.ref|{"id":"req-1"}' });
    await keys.revoke('aaaa');
    expect(keys.error.value).toEqual({ message: 'error.revoke_apikey' });
    expect(useAPIKeysStore().items.find(k => k.secretHint === 'aaaa')?.revokedAt).toBeUndefined();
    expect(useAuthStore().me).not.toBeNull();
  });
});
