import type { APIKey } from './apiKeys';
import { create } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { createPinia, setActivePinia } from 'pinia';

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { APIKeySchema } from '../../gen/apikey/v1/apikey_pb';
import { connectError, deferred, setClient, setKnownKeys } from '../../test/nuxt-globals';
import { toAPIKey, useAPIKeysStore } from './apiKeys';

const expires = new Date('2027-01-01T00:00:00Z');
const revoked = new Date('2026-06-01T00:00:00Z');
const keyPB = create(APIKeySchema, {
  kind: 'org',
  name: 'CI',
  scopes: ['agent:read'],
  projectId: '',
  projectIds: ['x6MH9WPWjktMwCBKHy4pF'],
  allProjects: false,
  secretHint: 'abcd',
  expiresAt: timestampFromDate(expires),
});
const key: APIKey = {
  rowKey: expect.any(String),
  kind: 'org',
  name: 'CI',
  scopes: ['agent:read'],
  projectId: '',
  projectIds: ['x6MH9WPWjktMwCBKHy4pF'],
  allProjects: false,
  secretHint: 'abcd',
  expiresAt: expires,
  revokedAt: undefined,
};

describe('toAPIKey', () => {
  it('converts timestamps to dates', () => {
    expect(toAPIKey(keyPB)).toEqual(key);
    const k = toAPIKey(create(APIKeySchema, { secretHint: 'abcd', revokedAt: timestampFromDate(revoked) }));
    expect(k.expiresAt).toBeUndefined();
    expect(k.revokedAt).toEqual(revoked);
  });

  it('gives every key its own row key, even when hints repeat', () => {
    expect(toAPIKey(keyPB).rowKey).not.toBe(toAPIKey(keyPB).rowKey);
  });
});

describe('useAPIKeysStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('loads the keys of an org', async () => {
    const listAPIKeys = vi.fn().mockResolvedValue({ keys: [keyPB] });
    setClient('$apiKeyClient', { listAPIKeys });
    const keys = useAPIKeysStore();
    await keys.load('V3StGXR8Z5jdHj6BmyT2a');
    expect(listAPIKeys).toHaveBeenCalledWith({ orgId: 'V3StGXR8Z5jdHj6BmyT2a' });
    expect(keys.items).toEqual([key]);
    expect(keys.loading).toBe(false);
  });

  it('treats a missing list as empty', async () => {
    setClient('$apiKeyClient', { listAPIKeys: vi.fn().mockResolvedValue({}) });
    const keys = useAPIKeysStore();
    await keys.load('V3StGXR8Z5jdHj6BmyT2a');
    expect(keys.items).toEqual([]);
  });

  it('shows a Connect error with its reference', async () => {
    setKnownKeys('apperror.GEN900');
    setClient('$apiKeyClient', { listAPIKeys: vi.fn().mockRejectedValue(connectError('GEN900')) });
    const keys = useAPIKeysStore();
    await keys.load('V3StGXR8Z5jdHj6BmyT2a');
    expect(keys.error).toEqual({ message: 'apperror.GEN900', reference: 'error.ref|{"id":"req-1"}' });
  });

  it('shows the fallback for a plain error', async () => {
    setClient('$apiKeyClient', { listAPIKeys: vi.fn().mockRejectedValue(new Error('boom')) });
    const keys = useAPIKeysStore();
    await keys.load('V3StGXR8Z5jdHj6BmyT2a');
    expect(keys.error).toEqual({ message: 'error.load_apikeys' });
  });

  it('shares an in-flight load and skips an org already loaded or empty', async () => {
    const pending = deferred<{ keys: typeof keyPB[] }>();
    const listAPIKeys = vi.fn(() => pending.promise);
    setClient('$apiKeyClient', { listAPIKeys });
    const keys = useAPIKeysStore();
    await keys.ensureLoaded('');
    const first = keys.ensureLoaded('V3StGXR8Z5jdHj6BmyT2a');
    const second = keys.ensureLoaded('V3StGXR8Z5jdHj6BmyT2a');
    expect(listAPIKeys).toHaveBeenCalledTimes(1);
    pending.resolve({ keys: [keyPB] });
    await Promise.all([first, second]);
    await keys.ensureLoaded('V3StGXR8Z5jdHj6BmyT2a');
    expect(listAPIKeys).toHaveBeenCalledTimes(1);
  });

  it('puts a new key first and replaces the live key with the same hint', () => {
    setClient('$apiKeyClient', {});
    const keys = useAPIKeysStore();
    const gone = { ...key, rowKey: 'r0', name: 'Old CI', revokedAt: revoked };
    keys.add(gone);
    keys.add({ ...key, rowKey: 'r1' });
    keys.add({ ...key, rowKey: 'r2', secretHint: 'wxyz', name: 'Deploy' });
    keys.add({ ...key, rowKey: 'r3', name: 'CI 2' });
    expect(keys.items.map(k => [k.rowKey, k.secretHint, k.name])).toEqual([['r3', 'abcd', 'CI 2'], ['r2', 'wxyz', 'Deploy'], ['r0', 'abcd', 'Old CI']]);
  });

  it('marks only the live key with a hint revoked', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-10-01T00:00:00Z'));
    setClient('$apiKeyClient', {});
    const keys = useAPIKeysStore();
    keys.add({ ...key, rowKey: 'r0', revokedAt: revoked });
    keys.add({ ...key, rowKey: 'r1', secretHint: 'wxyz' });
    keys.add({ ...key, rowKey: 'r2' });
    keys.markRevoked('abcd');
    expect(keys.items.map(k => [k.rowKey, k.revokedAt])).toEqual([['r2', new Date('2026-10-01T00:00:00Z')], ['r1', undefined], ['r0', revoked]]);
  });
});
