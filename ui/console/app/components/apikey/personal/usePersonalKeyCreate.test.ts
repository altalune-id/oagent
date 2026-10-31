import type { Mock } from 'vitest';
import type { CreatedKey } from './usePersonalKeys';
import { create } from '@bufbuild/protobuf';
import { timestampDate } from '@bufbuild/protobuf/wkt';
import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { mcpSnippet } from '#base/lib/tokens/mcpSnippet';
import { APIKeySchema } from '#gen/apikey/v1/apikey_pb';
import { connectError, runtimeConfig, setClient, setKnownKeys } from '#test/nuxt-globals';
import { useAPIKeysStore } from '~/stores/apiKeys';
import { useAuthStore } from '~/stores/auth';
import { useProjectsStore } from '~/stores/projects';
import { usePersonalKeyCreate } from './usePersonalKeyCreate';

const orgId = 'V3StGXR8Z5jdHj6BmyT2a';
const now = new Date('2026-10-01T00:00:00Z');

describe('usePersonalKeyCreate', () => {
  let createAPIKey: ReturnType<typeof vi.fn>;
  let onCreated: Mock<(created: CreatedKey) => void>;

  beforeEach(async () => {
    setActivePinia(createPinia());
    vi.useFakeTimers();
    vi.setSystemTime(now);
    vi.stubGlobal('window', { location: { origin: 'https://console.test' } });
    createAPIKey = vi.fn().mockResolvedValue({ key: create(APIKeySchema, { secretHint: 'cccc', kind: 'personal', name: 'Laptop' }), plaintext: 'oak_new' });
    onCreated = vi.fn();
    setClient('$apiKeyClient', { createAPIKey });
    setClient('$authClient', { whoami: vi.fn().mockResolvedValue({ source: 'local', userId: 'F5pLm2XwQ8kRt3VnZ7bYc', permissions: ['agent:*', 'member:write'] }) });
    setClient('$projectClient', { listProjects: vi.fn().mockResolvedValue({ projects: [{ id: 'x6MH9WPWjktMwCBKHy4pF', orgId, slug: 'web', name: 'Web' }, { id: 'KdrF3t3FHtppBPmMgj4sy', orgId: 'YeDxg2kMYznPJJG3wVmjj', slug: 'x', name: 'X' }] }) });
    await Promise.all([useAuthStore().load(), useProjectsStore().ensureLoaded(orgId)]);
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('starts empty, 30 days out, for every project', () => {
    const form = usePersonalKeyCreate(orgId, onCreated);
    expect(form.expiresInDays.value).toBe(30);
    expect(form.expiryOptions.map(o => o.days)).toEqual([7, 30, 90, 365]);
    expect(form.allProjects.value).toBe(true);
    expect(form.canSubmit.value).toBe(false);
    form.name.value = '  ';
    expect(form.canSubmit.value).toBe(false);
    form.name.value = 'Laptop';
    expect(form.canSubmit.value).toBe(true);
  });

  it('offers only the scopes the viewer holds and the org\'s projects', () => {
    const form = usePersonalKeyCreate(orgId, onCreated);
    expect(form.mintable.value).toEqual([
      { name: 'agent:read', label: 'scope.agent_read' },
      { name: 'agent:write', label: 'scope.agent_write' },
    ]);
    expect(form.projects.value).toEqual([{ id: 'x6MH9WPWjktMwCBKHy4pF', name: 'Web' }]);
  });

  it('creates a key for all projects, hands over the secret once and resets', async () => {
    runtimeConfig.public.apiUrl = '/base/api';
    const form = usePersonalKeyCreate(orgId, onCreated);
    form.name.value = ' Laptop ';
    form.scopes.value = ['agent:read'];
    form.expiresInDays.value = 7;
    form.projectIds.value = ['x6MH9WPWjktMwCBKHy4pF'];
    await form.submit();
    const req = createAPIKey.mock.calls[0]![0];
    expect(req).toMatchObject({ orgId, kind: 'personal', name: 'Laptop', scopes: ['agent:read'], allProjects: true, projectIds: [] });
    expect(timestampDate(req.expiresAt)).toEqual(new Date(now.getTime() + 7 * 86_400_000));
    expect(useAPIKeysStore().items.map(k => k.secretHint)).toEqual(['cccc']);
    expect(onCreated).toHaveBeenCalledWith({ plaintext: 'oak_new', snippet: mcpSnippet('oagent', 'https://console.test/base/mcp', 'oak_new') });
    expect(form.name.value).toBe('');
    expect(form.scopes.value).toEqual([]);
    expect(form.expiresInDays.value).toBe(30);
    expect(form.busy.value).toBe(false);
  });

  it('sends chosen projects and skips the store when no key comes back', async () => {
    createAPIKey.mockResolvedValue({ plaintext: 'oak_x' });
    const form = usePersonalKeyCreate(orgId, onCreated);
    form.name.value = 'Laptop';
    form.allProjects.value = false;
    form.projectIds.value = ['x6MH9WPWjktMwCBKHy4pF'];
    await form.submit();
    expect(createAPIKey.mock.calls[0]![0]).toMatchObject({ allProjects: false, projectIds: ['x6MH9WPWjktMwCBKHy4pF'] });
    expect(useAPIKeysStore().items).toEqual([]);
    expect(onCreated.mock.calls[0]![0].plaintext).toBe('oak_x');
  });

  it('sends nothing without a name', async () => {
    const form = usePersonalKeyCreate(orgId, onCreated);
    await form.submit();
    expect(createAPIKey).not.toHaveBeenCalled();
  });

  it('shows a refusal and keeps what was typed', async () => {
    setKnownKeys('apperror.GEN007');
    createAPIKey.mockRejectedValueOnce(connectError('GEN007', { permission: 'agent:write' })).mockRejectedValueOnce(new Error('boom'));
    const form = usePersonalKeyCreate(orgId, onCreated);
    form.name.value = 'Laptop';
    await form.submit();
    expect(form.error.value).toEqual({ message: 'apperror.GEN007|{"permission":"scope.agent_write"}', reference: 'error.ref|{"id":"req-1"}' });
    expect(form.name.value).toBe('Laptop');
    await form.submit();
    expect(form.error.value).toEqual({ message: 'error.create_apikey' });
    expect(onCreated).not.toHaveBeenCalled();
    expect(form.busy.value).toBe(false);
  });
});
